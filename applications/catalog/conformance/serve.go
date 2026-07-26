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
	"strings"
	"testing"
	"time"
)

// Serving profiles (monitor, control, rest, knowledge-manager, bench) launch an
// HTTP server and reach a terminal state only after a client posts a
// lifecycle/control event. Run() is synchronous and cannot drive them, so this
// file adds async launch plus HTTP control: Serve returns a handle the test
// pokes with WaitHealthy/Post and then drains with WaitExit.

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
	defer resp.Body.Close()
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

// CopyShippedProfile copies the whole directory tree of a shipped profile into
// a temp dir so a family test can run the wrapper an operator actually ships
// rather than a synthesized reconstruction. relProfile is the profile path
// relative to the agent-profiles root (e.g. "agents/monitor/profile.yaml").
//
// The entire profile directory is copied recursively, so the profile's relative
// references (machine.yaml, tools.yaml, rest.yaml, and any subdir such as
// openapi/ or llm/) resolve from the copy exactly as agent-core resolves them in
// production. The /opt/agent-core tool_config_dirs the shipped profile hard-codes
// need no patching: --core-root remaps them onto the checkout
// (spec.SetAgentCoreInstallRoot).
//
// patches applies exact string replacements across every copied file, for the
// few values the harness must control (chiefly hard-coded listen addresses). It
// patches only the named fields — it never rebuilds the machine, tools, or
// wrapper — and returns the path to the copied profile.yaml.
func CopyShippedProfile(t *testing.T, relProfile string, patches map[string]string) string {
	t.Helper()
	srcProfile := ProfilePath(relProfile)
	srcDir := filepath.Dir(srcProfile)
	dstDir := t.TempDir()

	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		for old, replacement := range patches {
			content = strings.ReplaceAll(content, old, replacement)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, []byte(content), 0o644)
	})
	if err != nil {
		t.Fatalf("copy shipped profile %s: %v", relProfile, err)
	}
	return filepath.Join(dstDir, filepath.Base(srcProfile))
}
