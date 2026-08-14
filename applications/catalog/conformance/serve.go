// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Serving profiles (monitor, control, rest, knowledge-manager, bench) launch an
// HTTP server and reach a terminal state only after a client posts a
// lifecycle/control event. Run() is synchronous and cannot drive them, so this
// file adds async launch plus HTTP control: Serve returns a handle the test
// pokes with WaitHealthy/Post and then drains with WaitExit.
//
// This is test-runner orchestration, not an agent workflow implementation. It
// deliberately observes the built CLI through process and HTTP boundaries;
// driving conformance through the product's own rest/service words would make
// the proof circular and lose the independent process-death watchdog (#1388).

const (
	defaultHealthTimeout = 15 * time.Second
	defaultExitTimeout   = 15 * time.Second
)

// FreeAddr reserves a loopback address whose port the OS just assigned and
// released, so a serving profile bound to it does not collide. There is an
// inherent bind race, which callers absorb by retrying via WaitHealthy.
func FreeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

// PortOf returns the port component of a host:port address.
func PortOf(t *testing.T, addr string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host:port %q: %v", addr, err)
	}
	return port
}

// ServeConfig configures an async (serving) profile launch.
type ServeConfig struct {
	Profile   string
	Directory string
	Args      []string
	Env       []string
	WorkDir   string
}

// Server is a running serving profile plus its trace destination.
type Server struct {
	t       *testing.T
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	out     *bytes.Buffer
	logFile string
	done    chan struct{}
	exitErr error
}

// Serve builds the agent binary and launches a serving profile asynchronously
// with --otel-log-file. It skips the test when the sibling agent-core checkout is absent. The
// process is killed on test cleanup if still running.
func Serve(t *testing.T, cfg ServeConfig) *Server {
	t.Helper()
	coreRoot := RequireCoreRoot(t)
	binary := agentBinary(t, coreRoot)

	profile := cfg.Profile
	if profile != "" && !filepath.IsAbs(profile) {
		profile = ProfilePath(profile)
	}
	logFile := filepath.Join(t.TempDir(), "trace.otel.json")
	args := []string{"--profile", profile, "--core-root", coreRoot, "--otel-log-file", logFile}
	if cfg.Directory != "" {
		args = append(args, "--directory", cfg.Directory)
	}
	args = append(args, cfg.Args...)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cfg.WorkDir
	if cmd.Dir == "" {
		cmd.Dir = ProfilesRoot()
	}
	cmd.Env = append(os.Environ(), cfg.Env...)
	out := &bytes.Buffer{}
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start serving profile: %v\nargs: %v", err, args)
	}
	s := &Server{t: t, cmd: cmd, cancel: cancel, out: out, logFile: logFile, done: make(chan struct{})}
	go func() {
		s.exitErr = cmd.Wait()
		close(s.done)
	}()
	t.Cleanup(s.Stop)
	return s
}

// WaitHealthy polls a GET url until it returns 200 or the timeout elapses. It
// fails fast if the server exits before becoming healthy.
func (s *Server) WaitHealthy(url string, timeout time.Duration) {
	s.t.Helper()
	if timeout <= 0 {
		timeout = defaultHealthTimeout
	}
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		select {
		case <-s.done:
			s.t.Fatalf("server exited before healthy at %s: %v\noutput:\n%s", url, s.exitErr, s.out.String())
		default:
		}
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.t.Fatalf("server not healthy at %s within %s: %v\noutput:\n%s", url, timeout, last, s.out.String())
}

// Post sends a JSON POST to url and returns the response status code.
func (s *Server) Post(url, body string) int {
	s.t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		s.t.Fatalf("build POST %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("POST %s: %v\noutput:\n%s", url, err, s.out.String())
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// WaitExit waits for the serving profile to terminate, then parses its trace
// and returns the run result.
func (s *Server) WaitExit(timeout time.Duration) RunResult {
	s.t.Helper()
	if timeout <= 0 {
		timeout = defaultExitTimeout
	}
	select {
	case <-s.done:
	case <-time.After(timeout):
		s.t.Fatalf("serving profile did not exit within %s\noutput:\n%s", timeout, s.out.String())
	}
	exitCode := 0
	if s.exitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(s.exitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			s.t.Fatalf("serving profile wait failed: %v\noutput:\n%s", s.exitErr, s.out.String())
		}
	}
	spans, err := ParseSpansFile(s.logFile)
	if err != nil {
		s.t.Fatalf("parse trace: %v\nexit=%d output:\n%s", err, exitCode, s.out.String())
	}
	return RunResult{Spans: spans, ExitCode: exitCode, Output: s.out.String(), LogFile: s.logFile}
}

// Stop cancels the process context and waits briefly for it to exit. Safe to
// call more than once.
func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

// writeEphemeral writes content to name under dir and returns the path.
func writeEphemeral(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// rewriteFile reads path and returns its contents with each replacement
// applied, for binding a fixed port into a profile's REST config.
func rewriteFile(t *testing.T, path string, replacements map[string]string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)
	for old, replacement := range replacements {
		content = strings.ReplaceAll(content, old, replacement)
	}
	return content
}

// CopyShippedProfile stages a shipped profile under its catalog-relative path
// so a family test can run the wrapper an operator actually ships rather than
// a synthesized reconstruction. relProfile is relative to the catalog root
// (e.g. "agents/runtime-state-reader/profile.yaml").
//
// The requested profile directory is copied recursively. YAML references that
// leave that directory are then copied transitively under the same
// catalog-relative paths; a referenced sibling profile brings its whole family
// directory. This preserves package-root references such as
// agents/critic/profile.yaml without copying unrelated generated catalog
// artifacts. /opt/agent-core references need no staging because --core-root
// remaps them onto the checkout (spec.SetAgentCoreInstallRoot).
//
// patches applies simultaneous exact string replacements only within the
// requested family for the few values the harness must control (chiefly
// hard-coded listen addresses). Transitive dependencies remain byte-identical.
func CopyShippedProfile(t *testing.T, relProfile string, patches map[string]string) string {
	t.Helper()
	root := ProfilesRoot()
	srcProfile := filepath.Clean(ProfilePath(relProfile))
	if err := requirePathWithin(root, srcProfile); err != nil {
		t.Fatalf("copy shipped profile %s: %v", relProfile, err)
	}
	replacer, err := newProfileReplacer(patches)
	if err != nil {
		t.Fatalf("copy shipped profile %s: %v", relProfile, err)
	}
	stage := &shippedProfileStage{
		sourceRoot: root,
		targetRoot: t.TempDir(),
		patchRoot:  filepath.Dir(srcProfile),
		replacer:   replacer,
		copied:     make(map[string]bool),
	}
	if err := stage.copyClosure(filepath.Dir(srcProfile)); err != nil {
		t.Fatalf("copy shipped profile %s: %v", relProfile, err)
	}
	relative, _ := filepath.Rel(root, srcProfile)
	return filepath.Join(stage.targetRoot, relative)
}

type shippedProfileStage struct {
	sourceRoot string
	targetRoot string
	patchRoot  string
	replacer   *strings.Replacer
	copied     map[string]bool
	pending    []string
}

var shippedProfileTemplatePattern = regexp.MustCompile(`\$\{[^}\r\n]+\}`)

func (s *shippedProfileStage) copyClosure(initial string) error {
	s.pending = append(s.pending, initial)
	for len(s.pending) > 0 {
		source := s.pending[0]
		s.pending = s.pending[1:]
		if s.copied[source] {
			continue
		}
		info, err := os.Lstat(source)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := s.copyDirectory(source); err != nil {
				return err
			}
			continue
		}
		if info.Mode().IsRegular() {
			if err := s.copyFile(source); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *shippedProfileStage) copyDirectory(source string) error {
	s.copied[source] = true
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return s.copyFile(path)
	})
}

func (s *shippedProfileStage) copyFile(source string) error {
	source = filepath.Clean(source)
	if s.copied[source] {
		return nil
	}
	if err := requirePathWithin(s.sourceRoot, source); err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(s.sourceRoot, source)
	if err != nil {
		return err
	}
	target := filepath.Join(s.targetRoot, relative)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	content := string(data)
	if requirePathWithin(s.patchRoot, source) == nil {
		content = s.replacer.Replace(content)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return err
	}
	s.copied[source] = true
	if filepath.Ext(source) == ".yaml" || filepath.Ext(source) == ".yml" {
		return s.enqueueYAMLReferences(source, data)
	}
	return nil
}

func (s *shippedProfileStage) enqueueYAMLReferences(source string, data []byte) error {
	var document yaml.Node
	inspectable := shippedProfileTemplatePattern.ReplaceAll(data, []byte("template_value"))
	if err := yaml.Unmarshal(inspectable, &document); err != nil {
		return fmt.Errorf("parse YAML dependencies in %s: %w", source, err)
	}
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Kind == yaml.ScalarNode {
			if dependency := s.resolveYAMLDependency(source, node.Value); dependency != "" {
				if isProfileFilename(filepath.Base(dependency)) {
					dependency = filepath.Dir(dependency)
				}
				s.pending = append(s.pending, dependency)
			}
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(&document)
	return nil
}

func (s *shippedProfileStage) resolveYAMLDependency(source, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "*?[") ||
		strings.HasPrefix(value, "/opt/agent-core/") {
		return ""
	}
	pathLike := strings.HasSuffix(value, ".yaml") || strings.HasSuffix(value, ".yml") ||
		strings.HasPrefix(value, "agents/") || strings.HasPrefix(value, "testdata/") ||
		strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../")
	if !pathLike || filepath.IsAbs(value) {
		return ""
	}
	var candidate string
	if strings.HasPrefix(value, "agents/") || strings.HasPrefix(value, "testdata/") {
		candidate = filepath.Join(s.sourceRoot, filepath.FromSlash(value))
	} else {
		candidate = filepath.Join(filepath.Dir(source), filepath.FromSlash(value))
	}
	candidate = filepath.Clean(candidate)
	if requirePathWithin(s.sourceRoot, candidate) != nil {
		return ""
	}
	if info, err := os.Stat(candidate); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
		return candidate
	}
	return ""
}

func requirePathWithin(root, candidate string) error {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return fmt.Errorf("compare catalog path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return fmt.Errorf("path escapes catalog root: %s", candidate)
	}
	return nil
}

func isProfileFilename(name string) bool {
	return name == "profile.yaml" ||
		strings.HasPrefix(name, "profile-") && strings.HasSuffix(name, ".yaml") ||
		strings.HasSuffix(name, "-profile.yaml")
}

func newProfileReplacer(patches map[string]string) (*strings.Replacer, error) {
	keys := make([]string, 0, len(patches))
	for old := range patches {
		if old == "" {
			return nil, fmt.Errorf("profile patch contains an empty match")
		}
		keys = append(keys, old)
	}
	sort.Strings(keys)
	for i, key := range keys {
		for _, other := range keys[i+1:] {
			if strings.Contains(other, key) {
				return nil, fmt.Errorf("profile patches %q and %q overlap", key, other)
			}
		}
	}
	pairs := make([]string, 0, len(keys)*2)
	for _, old := range keys {
		pairs = append(pairs, old, patches[old])
	}
	return strings.NewReplacer(pairs...), nil
}
