// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	observabilityStateDir  = "observability/.run"
	observabilityPidFile   = "collector.pid"
	observabilityLogFile   = "collector.log"
	observabilitySpoolDir  = "spool"
	collectorProfileRel    = "agents/collector/profile.yaml"
	observabilityStartWait = 20 * time.Second
	observabilityStopWait  = 15 * time.Second
)

var (
	startCollectorProcess   = startCollector
	stopCollectorProcess    = stopCollector
	checkObservability      = observabilityHealth
	checkObservabilityPort  = portAvailable
	collectorAlreadyRunning = collectorRunning
)

// Observability runs the persistent collector-agent ingress as a host process.
// The spool outlives any one integration run: down stops the process and keeps
// the spooled evidence, and only reset deletes it.
type Observability mg.Namespace

// Up starts the collector-agent ingress or reuses an already healthy one.
func (Observability) Up() error {
	if checkObservability() == nil {
		fmt.Println("collector ingress already healthy; reusing it")
		return nil
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
	return []namedPort{
		{"OTLP gRPC", envOrDefault("DA_OTEL_GRPC_PORT", "4317")},
		{"Collector control", envOrDefault("DA_COLLECTOR_CONTROL_PORT", "18191")},
		{"Collector query", envOrDefault("DA_COLLECTOR_QUERY_PORT", "18193")},
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
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(root, "agent-core"))
	if !agentCoreAvailable(coreRoot) {
		return fmt.Errorf("agent-core checkout not found at %s; set %s", coreRoot, agentCoreRootEnv)
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

func collectorEnviron(spoolDir string) []string {
	return []string{
		"COLLECTOR_MODE=spool",
		"COLLECTOR_BIND_HOST=" + envOrDefault("DA_COLLECTOR_BIND_HOST", "0.0.0.0"),
		"COLLECTOR_RECEIVER_ADDRESS=0.0.0.0:" + envOrDefault("DA_OTEL_GRPC_PORT", "4317"),
		"COLLECTOR_CONTROL_PORT=" + envOrDefault("DA_COLLECTOR_CONTROL_PORT", "18191"),
		"COLLECTOR_MONITOR_PORT=" + envOrDefault("DA_COLLECTOR_MONITOR_PORT", "18192"),
		"COLLECTOR_QUERY_PORT=" + envOrDefault("DA_COLLECTOR_QUERY_PORT", "18193"),
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
		fmt.Println("collector ingress is not running")
		return nil
	}
	if err := postCollectorExit(); err != nil {
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

func postCollectorExit() error {
	url := "http://127.0.0.1:" + envOrDefault("DA_COLLECTOR_CONTROL_PORT", "18191") + "/api/lifecycle/exit"
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
	checks := []struct {
		name string
		url  string
	}{
		{"Collector control", "http://127.0.0.1:" + envOrDefault("DA_COLLECTOR_CONTROL_PORT", "18191") + "/api/lifecycle/health"},
		{"Collector query", "http://127.0.0.1:" + envOrDefault("DA_COLLECTOR_QUERY_PORT", "18193") + "/query/traces"},
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

func commandOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
