// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"crypto/sha256"
	"errors"
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

	collectorReconciliationLockDir  = "declarative-agents"
	collectorReconciliationLockFile = "collector-reconciliation.lock"
	collectorReconciliationWait     = 30 * time.Second
	collectorReconciliationPoll     = 50 * time.Millisecond
)

var errCollectorNotFound = errors.New("collector process not found")

var (
	startCollectorProcess        = startCollector
	stopCollectorProcess         = stopCollector
	checkObservability           = observabilityHealth
	checkObservabilityPort       = portAvailable
	configuredObservabilityPorts = observabilityPorts
	currentCollectorFingerprint  = collectorSourceFingerprint
	readCollectorFingerprint     = readSourceFingerprint
	writeCollectorFingerprint    = writeSourceFingerprint
	locateCollectorProcess       = locateCollector
	requestCollectorExit         = postCollectorExit
	collectorProcessAlive        = processAlive
	terminateCollectorProcess    = terminateCollectorProcessGroup
	collectorCommandOutput       = commandOutput
	collectorSignalProcess       = syscall.Kill
	untrackedCollectorStopWait   = observabilityStopWait
	currentCollectorSource       = expectedCollectorSource
	runCollectorReconciliation   = withCollectorReconciliationLock
	collectorReconciliationPath  = gitCommonCollectorLockPath
	collectorReconciliationSleep = time.Sleep
	readCollectorProcessState    = collectorProcessState
)

var observabilityOutput io.Writer = os.Stdout

// Observability runs the persistent collector-agent ingress as a host process.
// The spool outlives any one integration run: down stops the process and keeps
// the spooled evidence, and only reset deletes it.
type Observability mg.Namespace

// Up starts the collector-agent ingress or reuses an already healthy one.
func (Observability) Up() error {
	return runCollectorReconciliation("up", observabilityUp)
}

func observabilityUp() error {
	fingerprint, err := currentCollectorFingerprint()
	if err != nil {
		return err
	}
	healthErr := checkObservability()
	if healthErr == nil {
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
	if healthErr != nil {
		process, _, locateErr := locateCollectorProcess()
		switch {
		case locateErr == nil:
			fmt.Printf("collector ingress is unhealthy but still owned by pid %d; replacing it\n",
				process.PID)
			if err := stopCollectorProcess(); err != nil {
				return err
			}
		case !errors.Is(locateErr, errCollectorNotFound):
			return fmt.Errorf("diagnose unhealthy collector ingress: %w", locateErr)
		}
	}
	for _, port := range configuredObservabilityPorts() {
		if err := checkObservabilityPort(port.name, port.value); err != nil {
			return err
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
	return runCollectorReconciliation("down", stopCollectorProcess)
}

// Reset stops the collector ingress and deletes the retained spool.
func (Observability) Reset() error {
	return runCollectorReconciliation("reset", func() error {
		if err := stopCollectorProcess(); err != nil {
			return err
		}
		spool := filepath.Join(observabilityStateDir, observabilitySpoolDir)
		if err := os.RemoveAll(spool); err != nil {
			return fmt.Errorf("remove collector spool %s: %w", spool, err)
		}
		fmt.Printf("+ removed collector spool %s\n", spool)
		return nil
	})
}

// Status reports whether the collector ingress is running and healthy.
func (Observability) Status() error {
	process, tracked, err := locateCollectorProcess()
	if err != nil {
		if errors.Is(err, errCollectorNotFound) {
			fmt.Fprintln(observabilityOutput, "collector ingress is not running")
			return checkObservability()
		}
		fmt.Fprintf(observabilityOutput, "collector ingress ownership error: %v\n", err)
		return errors.Join(err, checkObservability())
	}
	ownership := "listener-discovered"
	if tracked {
		ownership = "pid-tracked"
	}
	fmt.Fprintf(observabilityOutput, "collector ingress owner: pid %d (%s)\n", process.PID, ownership)
	fmt.Fprintf(observabilityOutput, "collector command: %s\n", process.Command)
	fmt.Fprintln(observabilityOutput, collectorSourceDescription(process))
	if err := checkObservability(); err != nil {
		fmt.Fprintf(observabilityOutput, "collector ingress unhealthy: %v\n", err)
		return err
	}
	fmt.Fprintln(observabilityOutput, "collector ingress healthy")
	return nil
}

type collectorProcess struct {
	PID       int
	Command   string
	Profile   string
	Directory string
	CoreRoot  string
}

type collectorSource struct {
	Profile   string
	Directory string
	CoreRoot  string
}

type namedPort struct {
	name  string
	value string
}

type collectorLifecycleLock struct {
	path string
	file *os.File
}

func withCollectorReconciliationLock(
	action string,
	run func() error,
) (result error) {
	path, err := collectorReconciliationPath()
	if err != nil {
		return fmt.Errorf("resolve collector reconciliation lock for %s: %w", action, err)
	}
	lock, err := acquireCollectorLifecycleLock(path, action, collectorReconciliationWait)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, lock.release())
	}()
	return run()
}

func acquireCollectorLifecycleLock(
	path, action string,
	timeout time.Duration,
) (*collectorLifecycleLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create collector reconciliation lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open collector reconciliation lock %s: %w", path, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			if truncateErr := file.Truncate(0); truncateErr != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
				return nil, fmt.Errorf("truncate collector reconciliation lock: %w", truncateErr)
			}
			if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
				return nil, fmt.Errorf("seek collector reconciliation lock: %w", seekErr)
			}
			cwd, _ := os.Getwd()
			if _, writeErr := fmt.Fprintf(file,
				"pid=%d action=%s cwd=%s acquired=%s\n",
				os.Getpid(), action, cwd, time.Now().UTC().Format(time.RFC3339Nano)); writeErr != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
				return nil, fmt.Errorf("write collector reconciliation lock metadata: %w", writeErr)
			}
			return &collectorLifecycleLock{path: path, file: file}, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			_ = file.Close()
			return nil, fmt.Errorf("acquire collector reconciliation lock %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			_, _ = file.Seek(0, io.SeekStart)
			holder, _ := io.ReadAll(file)
			_ = file.Close()
			return nil, fmt.Errorf(
				"collector reconciliation %s timed out after %s waiting for %s (holder: %s)",
				action, timeout, path, strings.TrimSpace(string(holder)))
		}
		collectorReconciliationSleep(collectorReconciliationPoll)
	}
}

func (lock *collectorLifecycleLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		unlockErr = fmt.Errorf("release collector reconciliation lock %s: %w",
			lock.path, unlockErr)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("close collector reconciliation lock %s: %w",
			lock.path, closeErr)
	}
	return errors.Join(unlockErr, closeErr)
}

func gitCommonCollectorLockPath() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	commonDir, err := collectorCommandOutput("git", "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("resolve Git common directory: %w: %s",
			err, strings.TrimSpace(commonDir))
	}
	commonDir = strings.TrimSpace(commonDir)
	if commonDir == "" {
		return "", errors.New("Git common directory is empty")
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	commonDir, err = filepath.Abs(commonDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Clean(commonDir),
		collectorReconciliationLockDir, collectorReconciliationLockFile), nil
}

func observabilityPorts() []namedPort {
	return observabilityPortsFrom(demoObservability())
}

func observabilityPortsFrom(settings observabilitySettings) []namedPort {
	return []namedPort{
		{"OTLP gRPC", settings.OTELGRPCPort},
		{"Collector control", settings.ControlPort},
		{"Collector monitor", settings.MonitorPort},
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
	go func() {
		// Reap the detached child when it exits while this Mage process is still
		// alive. If Mage exits first, the collector remains in its own process
		// group and the operating system adopts it.
		_ = cmd.Wait()
	}()
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
	process, tracked, err := locateCollectorProcess()
	if err != nil {
		if errors.Is(err, errCollectorNotFound) {
			if clearErr := clearCollectorPid(); clearErr != nil {
				return clearErr
			}
			fmt.Println("collector ingress is not running")
			return nil
		}
		return fmt.Errorf("identify collector ingress: %w", err)
	}
	pid := process.PID
	if err := requestCollectorExit(); err != nil {
		fmt.Printf("warning: collector pid %d exit request failed (%v); signalling the verified process\n",
			pid, err)
		if err := terminateCollectorProcess(pid); err != nil {
			return err
		}
		return clearCollectorPid()
	}
	stopWait := untrackedCollectorStopWait
	if tracked {
		stopWait = observabilityStopWait
	}
	deadline := time.Now().Add(stopWait)
	for time.Now().Before(deadline) {
		if !collectorProcessAlive(pid) {
			return clearCollectorPid()
		}
		time.Sleep(250 * time.Millisecond)
	}
	fmt.Printf("warning: collector pid %d remained alive after %s; signalling its verified process group\n",
		pid, stopWait)
	if err := terminateCollectorProcess(pid); err != nil {
		return err
	}
	return clearCollectorPid()
}

// locateCollector identifies the process owning any configured collector
// listener. Listener evidence takes precedence over local PID metadata because
// each worktree has its own ignored state directory. It returns tracked=true
// only when this checkout's live PID metadata names the discovered process.
func locateCollector() (collectorProcess, bool, error) {
	trackedPID, tracked := readCollectorPid()
	listenerOwners := make(map[int][]namedPort)
	for _, port := range configuredObservabilityPorts() {
		owners, err := listenerPIDs(port.value)
		if err != nil {
			return collectorProcess{}, false,
				fmt.Errorf("%s port %s: %w", port.name, port.value, err)
		}
		if len(owners) > 1 {
			return collectorProcess{}, false,
				fmt.Errorf("%s port %s has %d listener owners, want at most one",
					port.name, port.value, len(owners))
		}
		if len(owners) == 1 {
			listenerOwners[owners[0]] = append(listenerOwners[owners[0]], port)
		}
	}
	if len(listenerOwners) > 1 {
		return collectorProcess{}, false, describeSplitListenerOwners(listenerOwners)
	}
	for pid := range listenerOwners {
		process, err := inspectCollectorProcess(pid)
		if err != nil {
			return collectorProcess{}, false, err
		}
		return process, tracked && trackedPID == pid, nil
	}
	if !tracked {
		return collectorProcess{}, false, errCollectorNotFound
	}
	process, err := inspectCollectorProcess(trackedPID)
	if err != nil {
		return collectorProcess{}, false,
			fmt.Errorf("stale collector PID metadata names an unrelated process: %w", err)
	}
	return process, true, nil
}

func inspectCollectorProcess(pid int) (collectorProcess, error) {
	command, err := collectorCommandOutput("ps", "-ww", "-p", strconv.Itoa(pid), "-o", "command=")
	if err != nil {
		return collectorProcess{}, fmt.Errorf("read pid %d command: %w", pid, err)
	}
	command = filepath.ToSlash(strings.TrimSpace(command))
	process := collectorProcess{PID: pid, Command: command}
	var ok bool
	if process.Profile, ok = collectorCommandFlag(command, "--profile"); !ok {
		return process, fmt.Errorf("pid %d is not an owned collector; command lacks --profile: %s",
			pid, command)
	}
	if process.Directory, ok = collectorCommandFlag(command, "--directory"); !ok {
		return process, fmt.Errorf("pid %d is not an owned collector; command lacks --directory: %s",
			pid, command)
	}
	if process.CoreRoot, ok = collectorCommandFlag(command, "--core-root"); !ok {
		return process, fmt.Errorf("pid %d is not an owned collector; command lacks --core-root: %s",
			pid, command)
	}
	process.Profile = filepath.Clean(process.Profile)
	process.Directory = filepath.Clean(process.Directory)
	process.CoreRoot = filepath.Clean(process.CoreRoot)
	expectedProfile := filepath.Join(process.Directory, filepath.FromSlash(collectorProfileRel))
	if process.Profile != expectedProfile {
		return process, fmt.Errorf(
			"pid %d is not an owned collector; profile %s is outside catalog root %s: %s",
			pid, process.Profile, process.Directory, command)
	}
	return process, nil
}

func collectorCommandFlag(command, name string) (string, bool) {
	fields := strings.Fields(command)
	for index, field := range fields {
		if field == name && index+1 < len(fields) {
			return strings.Trim(fields[index+1], `"'`), true
		}
		if strings.HasPrefix(field, name+"=") {
			return strings.Trim(strings.TrimPrefix(field, name+"="), `"'`), true
		}
	}
	return "", false
}

func describeSplitListenerOwners(owners map[int][]namedPort) error {
	pids := make([]int, 0, len(owners))
	for pid := range owners {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	var descriptions []string
	for _, pid := range pids {
		command, err := collectorCommandOutput("ps", "-ww", "-p", strconv.Itoa(pid), "-o", "command=")
		if err != nil {
			command = fmt.Sprintf("<command unavailable: %v>", err)
		}
		var ports []string
		for _, port := range owners[pid] {
			ports = append(ports, port.name+"="+port.value)
		}
		descriptions = append(descriptions,
			fmt.Sprintf("pid %d [%s] command=%s", pid, strings.Join(ports, ","),
				strings.TrimSpace(command)))
	}
	return fmt.Errorf("configured collector listeners have different owners: %s",
		strings.Join(descriptions, "; "))
}

func listenerPIDs(port string) ([]int, error) {
	output, err := collectorCommandOutput("lsof", "-nP", "-t", "-iTCP:"+port, "-sTCP:LISTEN")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 &&
			strings.TrimSpace(output) == "" {
			return nil, nil
		}
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
	if err := signalVerifiedCollector(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("terminate collector process group %d: %w", pid, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !collectorProcessAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := signalVerifiedCollector(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill collector process group %d: %w", pid, err)
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !collectorProcessAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if collectorProcessAlive(pid) {
		return fmt.Errorf("collector process group %d remained alive after SIGKILL", pid)
	}
	return nil
}

func signalVerifiedCollector(pid int, signal syscall.Signal) error {
	err := collectorSignalProcess(-pid, signal)
	if err == nil {
		return nil
	}
	if !collectorProcessAlive(pid) {
		return nil
	}
	if err != syscall.EPERM && err != syscall.ESRCH {
		return err
	}
	if _, verifyErr := inspectCollectorProcess(pid); verifyErr != nil {
		if !collectorProcessAlive(pid) {
			return nil
		}
		return fmt.Errorf("refuse direct signal after process-group error %v: %w", err, verifyErr)
	}
	if directErr := collectorSignalProcess(pid, signal); directErr != nil &&
		directErr != syscall.ESRCH {
		return directErr
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
	if state, err := readCollectorProcessState(pid); err == nil &&
		strings.HasPrefix(strings.TrimSpace(state), "Z") {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func collectorProcessState(pid int) (string, error) {
	return collectorCommandOutput(
		"ps", "-p", strconv.Itoa(pid), "-o", "stat=")
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

func expectedCollectorSource() (collectorSource, error) {
	root, err := os.Getwd()
	if err != nil {
		return collectorSource{}, fmt.Errorf("resolve collector working directory: %w", err)
	}
	catalogRoot, err := resolveCatalogRoot("observability source", root)
	if err != nil {
		return collectorSource{}, err
	}
	return collectorSource{
		Profile:   filepath.Join(catalogRoot, filepath.FromSlash(collectorProfileRel)),
		Directory: catalogRoot,
		CoreRoot:  demoCoreRoot(root),
	}, nil
}

func collectorSourceDescription(process collectorProcess) string {
	expected, err := currentCollectorSource()
	if err != nil {
		return fmt.Sprintf("collector source revision unknown: %v", err)
	}
	var mismatches []string
	if process.Profile != expected.Profile {
		mismatches = append(mismatches,
			fmt.Sprintf("profile=%s current=%s", process.Profile, expected.Profile))
	}
	if process.Directory != expected.Directory {
		mismatches = append(mismatches,
			fmt.Sprintf("catalog=%s current=%s", process.Directory, expected.Directory))
	}
	if process.CoreRoot != expected.CoreRoot {
		mismatches = append(mismatches,
			fmt.Sprintf("agent-core=%s current=%s", process.CoreRoot, expected.CoreRoot))
	}
	for _, source := range []struct {
		name string
		path string
	}{
		{"profile", process.Profile},
		{"catalog", process.Directory},
		{"agent-core", process.CoreRoot},
	} {
		if _, statErr := os.Stat(source.path); os.IsNotExist(statErr) {
			mismatches = append(mismatches,
				fmt.Sprintf("%s source was removed: %s", source.name, source.path))
		}
	}
	if len(mismatches) == 0 {
		current, currentErr := currentCollectorFingerprint()
		stored, storedErr := readCollectorFingerprint()
		switch {
		case currentErr != nil:
			mismatches = append(mismatches,
				fmt.Sprintf("current fingerprint unavailable: %v", currentErr))
		case storedErr != nil:
			mismatches = append(mismatches,
				fmt.Sprintf("launch fingerprint unavailable: %v", storedErr))
		case current != stored:
			mismatches = append(mismatches,
				fmt.Sprintf("fingerprint=%s current=%s", stored, current))
		default:
			return fmt.Sprintf("collector source matched: %s (%s)", process.Profile, current)
		}
	}
	return "collector source revision mismatch: " + strings.Join(mismatches, "; ")
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
