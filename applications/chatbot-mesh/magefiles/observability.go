// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/magefile/mage/mg"
)

// The persistent integration telemetry ingress is the canonical collector agent
// run as a background host process (srd008-telemetry R9, srd042 R8/R9). It
// receives both trace and metric OTLP exports on one gRPC listener and retains
// them in its spool, so no docker-compose stack, contrib collector, or
// Prometheus backend is required; kind remains the only Docker consumer.
const (
	observabilityStateDir   = "observability/.run"
	observabilityPidFile    = "collector.pid"
	observabilityLogFile    = "collector.log"
	observabilitySourceFile = "collector.source"
	observabilitySpoolDir   = "spool"
	collectorProfileRel     = "agents/collector/profile.yaml"
	observabilityStartWait  = 20 * time.Second
	observabilityStopWait   = 15 * time.Second
)

var (
	startCollectorProcess       = startCollector
	stopCollectorProcess        = stopCollector
	checkObservability          = observabilityHealth
	checkObservabilityPort      = portAvailable
	collectorAlreadyRunning     = collectorRunning
	currentCollectorFingerprint = collectorSourceFingerprint
	readCollectorFingerprint    = readSourceFingerprint
	writeCollectorFingerprint   = writeSourceFingerprint
	findUntrackedCollector      = discoverUntrackedCollector
	requestCollectorExit        = postCollectorExit
	collectorProcessAlive       = processAlive
	terminateCollectorProcess   = terminateCollectorProcessGroup
	collectorCommandOutput      = commandOutput
	untrackedCollectorStopWait  = observabilityStopWait
)

// Observability runs the persistent collector-agent ingress as a host process.
// The spool outlives any one integration run: down stops the process and keeps
// the spooled evidence, and only reset deletes it.
type Observability mg.Namespace

// Up starts the collector-agent ingress or reuses an already healthy one.
func (Observability) Up() error {
	fingerprint, err := currentCollectorFingerprint()
	if err != nil {
		return err
	}
	if checkObservability() == nil {
		stored, readErr := readCollectorFingerprint()
		if readErr == nil && stored == fingerprint {
			fmt.Println("collector ingress already healthy and source-matched; reusing it")
			return nil
		}
		fmt.Printf("collector ingress is healthy but stale (stored %q, current %q); restarting and preserving its spool\n",
			stored, fingerprint)
		if err := stopCollectorProcess(); err != nil {
			return err
		}
	}
	if !collectorAlreadyRunning() {
		for _, port := range observabilityPorts() {
			if err := checkObservabilityPort(port.name, port.value); err != nil {
				return err
			}
		}
	}
	if err := startCollectorProcess(); err != nil {
		return err
	}
	if err := writeCollectorFingerprint(fingerprint); err != nil {
		_ = stopCollectorProcess()
		return err
	}
	return waitObservabilityHealthy(observabilityStartWait)
}

// Down stops the collector ingress and keeps the spooled evidence.
func (Observability) Down() error {
	return stopCollectorProcess()
}

// Reset stops the collector ingress and deletes the retained spool.
func (Observability) Reset() error {
	if err := stopCollectorProcess(); err != nil {
		return err
	}
	spool := filepath.Join(observabilityStateDir, observabilitySpoolDir)
	if err := os.RemoveAll(spool); err != nil {
		return fmt.Errorf("remove collector spool %s: %w", spool, err)
	}
	fmt.Printf("+ removed collector spool %s\n", spool)
	return nil
}

// Status reports whether the collector ingress is running and healthy.
func (Observability) Status() error {
	pid, running := readCollectorPid()
	if !running {
		fmt.Println("collector ingress is not running")
		return checkObservability()
	}
	fmt.Printf("collector ingress running (pid %d)\n", pid)
	return checkObservability()
}

type namedPort struct {
	name  string
	value string
}

func observabilityPorts() []namedPort {
	return observabilityPortsFrom(demoObservability())
}

func observabilityPortsFrom(settings observabilitySettings) []namedPort {
	return []namedPort{
		{"OTLP gRPC", settings.OTELGRPCPort},
		{"Collector control", settings.ControlPort},
		{"Collector query", settings.QueryPort},
	}
}

// startCollector builds the agent binary from the agent-core checkout and
// launches the catalog collector profile as a detached background process, so
// it outlives the mage invocation that started it.
func startCollector() error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve collector working directory: %w", err)
	}
	coreRoot := demoCoreRoot(root)
	if !agentCoreAvailable(coreRoot) {
		return fmt.Errorf("agent-core checkout not found at %s; set core_root in demo.yaml", coreRoot)
	}
	catalogRoot, err := resolveCatalogRoot("observability collector", root)
	if err != nil {
		return err
	}
	profile := filepath.Join(catalogRoot, filepath.FromSlash(collectorProfileRel))
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	stateDir := filepath.Join(root, observabilityStateDir)
	spoolDir := filepath.Join(stateDir, observabilitySpoolDir)
	if err := os.MkdirAll(spoolDir, 0o755); err != nil {
		return fmt.Errorf("create collector state directory: %w", err)
	}
	logPath := filepath.Join(stateDir, observabilityLogFile)
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create collector log %s: %w", logPath, err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(binary,
		"--profile", profile, "--directory", catalogRoot, "--core-root", coreRoot)
	cmd.Env = append(os.Environ(), collectorEnviron(spoolDir)...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	fmt.Printf("+ starting collector ingress: %s --profile %s\n", binary, profile)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start collector ingress: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	if err := os.WriteFile(filepath.Join(stateDir, observabilityPidFile),
		[]byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("record collector pid: %w", err)
	}
	return nil
}

// collectorEnviron builds the collector child's process environment. The
// COLLECTOR_* variables are the collector profile's declared parameterization
// contract: agent-core expands ${VAR:-default} references in mounted
// declarations (srd013 R5.6/R5.7), so setting them here is the same act as a
// Helm chart setting pod env — using the collector's contract, not
// configuring the magefile. The magefile's own inputs come from demo.yaml.
func collectorEnviron(spoolDir string) []string {
	settings := demoObservability()
	return []string{
		"COLLECTOR_MODE=spool",
		"COLLECTOR_BIND_HOST=" + settings.BindHost,
		"COLLECTOR_RECEIVER_ADDRESS=0.0.0.0:" + settings.OTELGRPCPort,
		"COLLECTOR_CONTROL_PORT=" + settings.ControlPort,
		"COLLECTOR_MONITOR_PORT=" + settings.MonitorPort,
		"COLLECTOR_QUERY_PORT=" + settings.QueryPort,
		"COLLECTOR_SPOOL_PATH=" + filepath.Join(spoolDir, "traces", "collector.ndjson"),
		"COLLECTOR_METRICS_SPOOL_PATH=" + filepath.Join(spoolDir, "metrics", "collector.ndjson"),
	}
}

// stopCollector posts the lifecycle exit request and waits for the process to
// leave, preserving the spool. It falls back to signalling the process group
// when the control route does not stop it in time.
func stopCollector() error {
	pid, running := readCollectorPid()
	if !running {
		if checkObservability() != nil {
			fmt.Println("collector ingress is not running")
			return nil
		}
		// A healthy collector launched from another worktree has no PID in this
		// checkout's state directory. Verify that one collector process owns every
		// shared listener before asking it to exit; this gives us a safe bounded
		// signal fallback when an older lifecycle implementation accepts the
		// request but remains alive (GH-1496).
		pid, err := findUntrackedCollector()
		if err != nil {
			return fmt.Errorf("identify untracked collector ingress: %w", err)
		}
		if err := requestCollectorExit(); err != nil {
			fmt.Printf("warning: untracked collector exit request failed (%v); signalling the verified process\n", err)
			return terminateCollectorProcess(pid)
		}
		deadline := time.Now().Add(untrackedCollectorStopWait)
		for time.Now().Before(deadline) {
			if !collectorProcessAlive(pid) {
				return nil
			}
			time.Sleep(250 * time.Millisecond)
		}
		fmt.Printf("warning: untracked collector pid %d remained alive after %s; signalling its process group\n",
			pid, untrackedCollectorStopWait)
		return terminateCollectorProcess(pid)
	}
	if err := requestCollectorExit(); err != nil {
		fmt.Printf("warning: collector exit request failed (%v); signalling the process\n", err)
	}
	deadline := time.Now().Add(observabilityStopWait)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return clearCollectorPid()
		}
		time.Sleep(250 * time.Millisecond)
	}
	// The control exit did not stop it; terminate the process group.
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(time.Second)
	if processAlive(pid) {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	return clearCollectorPid()
}

// discoverUntrackedCollector returns the single process that owns every
// configured collector listener. It refuses ambiguous ownership and verifies
// the process command before stopCollector is allowed to signal it.
func discoverUntrackedCollector() (int, error) {
	pid := 0
	for _, port := range observabilityPorts() {
		owners, err := listenerPIDs(port.value)
		if err != nil {
			return 0, fmt.Errorf("%s port %s: %w", port.name, port.value, err)
		}
		if len(owners) != 1 {
			return 0, fmt.Errorf("%s port %s has %d listener owners, want one",
				port.name, port.value, len(owners))
		}
		if pid != 0 && owners[0] != pid {
			return 0, fmt.Errorf("collector listeners have different owners: pid %d and pid %d",
				pid, owners[0])
		}
		pid = owners[0]
	}
	command, err := collectorCommandOutput("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	if err != nil {
		return 0, fmt.Errorf("read pid %d command: %w", pid, err)
	}
	command = filepath.ToSlash(strings.TrimSpace(command))
	if !strings.Contains(command, "--profile") ||
		!strings.Contains(command, collectorProfileRel) {
		return 0, fmt.Errorf("pid %d does not run the collector profile: %s", pid, command)
	}
	return pid, nil
}

func listenerPIDs(port string) ([]int, error) {
	output, err := collectorCommandOutput("lsof", "-nP", "-t", "-iTCP:"+port, "-sTCP:LISTEN")
	if err != nil {
		return nil, fmt.Errorf("list listener processes: %w", err)
	}
	seen := make(map[int]struct{})
	var pids []int
	for _, line := range strings.Fields(output) {
		pid, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("parse listener pid %q: %w", line, err)
		}
		if _, exists := seen[pid]; exists {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids, nil
}

func terminateCollectorProcessGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("terminate collector process group %d: %w", pid, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("kill collector process group %d: %w", pid, err)
	}
	time.Sleep(50 * time.Millisecond)
	if processAlive(pid) {
		return fmt.Errorf("collector process group %d remained alive after SIGKILL", pid)
	}
	return nil
}

func postCollectorExit() error {
	url := "http://127.0.0.1:" + demoObservability().ControlPort + "/api/lifecycle/exit"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", strings.NewReader(`{"reason":"observability down"}`))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("collector exit route returned %s", resp.Status)
	}
	return nil
}

func collectorRunning() bool {
	_, running := readCollectorPid()
	return running
}

func readCollectorPid() (int, bool) {
	data, err := os.ReadFile(filepath.Join(observabilityStateDir, observabilityPidFile))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !processAlive(pid) {
		return 0, false
	}
	return pid, true
}

func clearCollectorPid() error {
	err := os.Remove(filepath.Join(observabilityStateDir, observabilityPidFile))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear collector pid: %w", err)
	}
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func portAvailable(name, port string) error {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err == nil {
		return listener.Close()
	}
	owner, _ := commandOutput("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN")
	if owner != "" {
		return fmt.Errorf("%s port %s is unavailable:\n%s", name, port, strings.TrimSpace(owner))
	}
	return fmt.Errorf("%s port %s is unavailable: %w", name, port, err)
}

// observabilityHealth verifies the collector control server is up and its query
// surface answers, proving both the ingress and the read path are live.
func observabilityHealth() error {
	settings := demoObservability()
	checks := []struct {
		name string
		url  string
	}{
		{"Collector control", "http://127.0.0.1:" + settings.ControlPort + "/api/lifecycle/health"},
		{"Collector query", "http://127.0.0.1:" + settings.QueryPort + "/query/traces"},
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, check := range checks {
		response, err := client.Get(check.url)
		if err != nil {
			return fmt.Errorf("%s health %s: %w", check.name, check.url, err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("%s health %s: HTTP %s", check.name, check.url, response.Status)
		}
	}
	return nil
}

func waitObservabilityHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if lastErr = checkObservability(); lastErr == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("collector ingress did not become healthy within %s: %w", timeout, lastErr)
}

// collectorSourceFingerprint hashes the production runtime source plus the
// collector's catalog closure. A healthy process may be reused only when this
// identity matches what was recorded at launch (GH-1492).
func collectorSourceFingerprint() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	coreRoot := demoCoreRoot(root)
	catalogRoot, err := resolveCatalogRoot("observability fingerprint", root)
	if err != nil {
		return "", err
	}
	paths := []struct {
		name string
		path string
	}{
		{"agent-core", coreRoot},
		{"collector", filepath.Join(catalogRoot, "agents", "collector")},
	}
	type sourceFile struct{ logical, path string }
	var files []sourceFile
	for _, root := range paths {
		path := root.path
		if err := filepath.WalkDir(path, func(file string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if file != path && skipCollectorFingerprintDir(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if collectorFingerprintFile(entry.Name()) {
				relative, err := filepath.Rel(path, file)
				if err != nil {
					return err
				}
				files = append(files, sourceFile{
					logical: filepath.ToSlash(filepath.Join(root.name, relative)),
					path:    file,
				})
			}
			return nil
		}); err != nil {
			return "", err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].logical < files[j].logical })
	hash := sha256.New()
	for _, file := range files {
		data, err := os.ReadFile(file.path)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(hash, "%s\x00", file.logical)
		_, _ = hash.Write(data)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

func skipCollectorFingerprintDir(name string) bool {
	switch name {
	case ".git", "bin", "build", "generated-files", "node_modules", "testdata":
		return true
	}
	return false
}

func collectorFingerprintFile(name string) bool {
	if strings.HasSuffix(name, "_test.go") {
		return false
	}
	switch filepath.Ext(name) {
	case ".go", ".yaml", ".yml":
		return true
	}
	return name == "go.mod" || name == "go.sum"
}

func readSourceFingerprint() (string, error) {
	data, err := os.ReadFile(filepath.Join(observabilityStateDir, observabilitySourceFile))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeSourceFingerprint(fingerprint string) error {
	stateDir := observabilityStateDir
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(stateDir, observabilitySourceFile)
	if err := os.WriteFile(path, []byte(fingerprint+"\n"), 0o644); err != nil {
		return fmt.Errorf("write collector source fingerprint: %w", err)
	}
	return nil
}

func commandOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
