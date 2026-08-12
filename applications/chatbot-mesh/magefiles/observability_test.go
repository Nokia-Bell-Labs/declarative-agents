// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestObservabilityUpReusesHealthyIngress(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return nil }
	startCollectorProcess = func() error {
		t.Fatal("healthy ingress must not start a new collector")
		return nil
	}

	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
}

func TestObservabilityUpRestartsHealthyStaleIngress(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return nil }
	currentCollectorFingerprint = func() (string, error) { return "current", nil }
	readCollectorFingerprint = func() (string, error) { return "old", nil }
	stopped, started, wrote := false, false, ""
	stopCollectorProcess = func() error {
		stopped = true
		checkObservability = func() error { return errors.New("stopped") }
		return nil
	}
	startCollectorProcess = func() error {
		started = true
		checkObservability = func() error { return nil }
		return nil
	}
	writeCollectorFingerprint = func(value string) error {
		wrote = value
		return nil
	}

	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
	if !stopped || !started || wrote != "current" {
		t.Fatalf("restart = stopped %v started %v wrote %q", stopped, started, wrote)
	}
}

func TestObservabilityUpRestartsWhenFingerprintIsMissing(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return nil }
	readCollectorFingerprint = func() (string, error) {
		return "", errors.New("fingerprint missing")
	}
	stops, starts := 0, 0
	stopCollectorProcess = func() error {
		stops++
		checkObservability = func() error { return errors.New("stopped") }
		return nil
	}
	startCollectorProcess = func() error {
		starts++
		checkObservability = func() error { return nil }
		return nil
	}
	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
	if stops != 1 || starts != 1 {
		t.Fatalf("stops/starts = %d/%d, want 1/1", stops, starts)
	}
}

func TestObservabilityUpStopsOnStaleRestartFailure(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return nil }
	readCollectorFingerprint = func() (string, error) { return "old", nil }
	stopCollectorProcess = func() error { return errors.New("controlled stop failure") }
	startCollectorProcess = func() error {
		t.Fatal("collector started after stale process failed to stop")
		return nil
	}
	err := (Observability{}).Up()
	if err == nil || !strings.Contains(err.Error(), "controlled stop failure") {
		t.Fatalf("error = %v, want controlled stop failure", err)
	}
}

func TestObservabilityUpPreservesSpoolDuringStaleRestart(t *testing.T) {
	restoreObservabilityHooks(t)
	t.Chdir(t.TempDir())
	spool := filepath.Join(observabilityStateDir, observabilitySpoolDir,
		"traces", "collector.ndjson")
	if err := os.MkdirAll(filepath.Dir(spool), 0o755); err != nil {
		t.Fatal(err)
	}
	const evidence = "retained trace evidence\n"
	if err := os.WriteFile(spool, []byte(evidence), 0o644); err != nil {
		t.Fatal(err)
	}
	checkObservability = func() error { return nil }
	readCollectorFingerprint = func() (string, error) { return "old", nil }
	stopCollectorProcess = func() error {
		assertFileContent(t, spool, evidence)
		checkObservability = func() error { return errors.New("stopped") }
		return nil
	}
	startCollectorProcess = func() error {
		assertFileContent(t, spool, evidence)
		checkObservability = func() error { return nil }
		return nil
	}

	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, spool, evidence)
}

func TestLocateCollectorDetectsPartialOwnedListenersWithoutMetadata(t *testing.T) {
	restoreObservabilityHooks(t)
	t.Chdir(t.TempDir())
	process := collectorFixtureProcess(4242, "/removed/worktree")
	collectorCommandOutput = func(name string, args ...string) (string, error) {
		if name == "lsof" {
			if strings.Contains(strings.Join(args, " "), ":"+collectorControlPortDefault) {
				return "", nil
			}
			return "4242\n", nil
		}
		return process.Command + "\n", nil
	}
	found, tracked, err := locateCollector()
	if err != nil {
		t.Fatal(err)
	}
	if found.PID != 4242 || tracked {
		t.Fatalf("collector = %+v tracked=%v, want listener-discovered pid 4242", found, tracked)
	}
}

func TestFixtureCollectorWithoutMetadataIsDetectedAndStopped(t *testing.T) {
	restoreObservabilityHooks(t)
	root := t.TempDir()
	t.Chdir(root)
	ready := filepath.Join(root, "fixture.ready")
	process := collectorFixtureProcess(0, filepath.Join(root, "removed-worktree"))
	command := exec.Command(
		os.Args[0], "-test.run=^TestCollectorListenerFixture$", "--",
		"--profile", process.Profile,
		"--directory", process.Directory,
		"--core-root", process.CoreRoot,
	)
	command.Env = append(os.Environ(),
		"GO_WANT_COLLECTOR_FIXTURE=1",
		"COLLECTOR_FIXTURE_READY="+ready,
	)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	pid := command.Process.Pid
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		select {
		case <-waitDone:
		case <-time.After(time.Second):
		}
	})

	var ports []string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(ready)
		if err == nil {
			ports = strings.Split(strings.TrimSpace(string(data)), ",")
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(ports) != 4 {
		t.Fatalf("fixture ports = %v, want four", ports)
	}
	configuredObservabilityPorts = func() []namedPort {
		return []namedPort{
			{name: "OTLP gRPC", value: ports[0]},
			{name: "Collector control", value: ports[1]},
			{name: "Collector monitor", value: ports[2]},
			{name: "Collector query", value: ports[3]},
		}
	}
	locateCollectorProcess = locateCollector
	var found collectorProcess
	var tracked bool
	var locateErr error
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		found, tracked, locateErr = locateCollector()
		if locateErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if locateErr != nil {
		var diagnostics []string
		for _, port := range ports {
			output, err := commandOutput("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN")
			diagnostics = append(diagnostics,
				port+": "+strings.TrimSpace(output)+" err="+errorString(err))
		}
		t.Fatalf("locate fixture: %v; %s", locateErr, strings.Join(diagnostics, "; "))
	}
	if found.PID != pid || tracked {
		t.Fatalf("collector = %+v tracked=%v, want fixture pid %d without metadata",
			found, tracked, pid)
	}
	requestCollectorExit = func() error {
		return syscall.Kill(-pid, syscall.SIGTERM)
	}
	untrackedCollectorStopWait = 5 * time.Second
	if err := stopCollector(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatalf("fixture collector pid %d remained alive", pid)
	}
}

func errorString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func TestCollectorListenerFixture(t *testing.T) {
	if os.Getenv("GO_WANT_COLLECTOR_FIXTURE") != "1" {
		return
	}
	var listeners []net.Listener
	var ports []string
	for range 4 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners = append(listeners, listener)
		_, port, err := net.SplitHostPort(listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		ports = append(ports, port)
	}
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	if err := os.WriteFile(
		os.Getenv("COLLECTOR_FIXTURE_READY"),
		[]byte(strings.Join(ports, ",")),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestLocateCollectorRejectsDifferentOwnersAndReportsCommands(t *testing.T) {
	restoreObservabilityHooks(t)
	t.Chdir(t.TempDir())
	collectorCommandOutput = func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if name == "ps" {
			if strings.Contains(joined, "4343") {
				return "/usr/bin/python unrelated.py\n", nil
			}
			return collectorFixtureProcess(4242, "/removed/worktree").Command + "\n", nil
		}
		if strings.Contains(joined, ":"+collectorControlPortDefault) {
			return "4343\n", nil
		}
		return "4242\n", nil
	}
	_, _, err := locateCollector()
	if err == nil ||
		!strings.Contains(err.Error(), "different owners") ||
		!strings.Contains(err.Error(), "unrelated.py") {
		t.Fatalf("error = %v, want owner commands", err)
	}
}

func TestLocateCollectorDiagnosesUnrelatedListenerOnSharedPorts(t *testing.T) {
	for _, port := range []string{
		otelGRPCPortDefault,
		collectorMonitorPortDefault,
		collectorQueryPortDefault,
	} {
		t.Run(port, func(t *testing.T) {
			restoreObservabilityHooks(t)
			t.Chdir(t.TempDir())
			collectorCommandOutput = func(name string, args ...string) (string, error) {
				if name == "ps" {
					return "/usr/bin/python unrelated_server.py\n", nil
				}
				if strings.Contains(strings.Join(args, " "), ":"+port) {
					return "4242\n", nil
				}
				return "", nil
			}
			_, _, err := locateCollector()
			if err == nil ||
				!strings.Contains(err.Error(), "not an owned collector") ||
				!strings.Contains(err.Error(), "unrelated_server.py") {
				t.Fatalf("error = %v, want unrelated listener diagnosis", err)
			}
		})
	}
}

func TestStopCollectorDoesNotTouchMonitorOnlyUnrelatedOwner(t *testing.T) {
	restoreObservabilityHooks(t)
	t.Chdir(t.TempDir())
	collectorCommandOutput = func(name string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if name == "ps" {
			return "/usr/bin/python unrelated_monitor.py\n", nil
		}
		if strings.Contains(joined, ":"+collectorMonitorPortDefault) {
			return "4343\n", nil
		}
		return "", nil
	}
	locateCollectorProcess = locateCollector
	requestCollectorExit = func() error {
		t.Fatal("monitor-only unrelated owner must not receive a lifecycle request")
		return nil
	}
	terminateCollectorProcess = func(int) error {
		t.Fatal("monitor-only unrelated owner must not be signalled")
		return nil
	}

	err := stopCollector()
	for _, want := range []string{"4343", "unrelated_monitor.py"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want monitor owner detail %q", err, want)
		}
	}
}

func TestMonitorOnlyUnrelatedListenerFixtureIsDiagnosedAndUntouched(t *testing.T) {
	restoreObservabilityHooks(t)
	root := t.TempDir()
	t.Chdir(root)
	ready := filepath.Join(root, "monitor.ready")
	command := exec.Command(
		os.Args[0], "-test.run=^TestUnrelatedMonitorListenerFixture$")
	command.Env = append(os.Environ(),
		"GO_WANT_UNRELATED_MONITOR_FIXTURE=1",
		"UNRELATED_MONITOR_READY="+ready,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	pid := command.Process.Pid
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-waitDone:
		case <-time.After(time.Second):
		}
	})
	var monitorPort string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(ready); err == nil {
			monitorPort = strings.TrimSpace(string(data))
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if monitorPort == "" {
		t.Fatal("unrelated monitor fixture did not become ready")
	}
	otherPorts, err := reserveLoopbackPorts(3)
	if err != nil {
		t.Fatal(err)
	}
	configuredObservabilityPorts = func() []namedPort {
		return []namedPort{
			{name: "OTLP gRPC", value: strconv.Itoa(otherPorts[0])},
			{name: "Collector control", value: strconv.Itoa(otherPorts[1])},
			{name: "Collector monitor", value: monitorPort},
			{name: "Collector query", value: strconv.Itoa(otherPorts[2])},
		}
	}
	locateCollectorProcess = locateCollector
	requestCollectorExit = func() error {
		t.Fatal("unrelated monitor fixture must not receive a lifecycle request")
		return nil
	}
	terminateCollectorProcess = func(int) error {
		t.Fatal("unrelated monitor fixture must not be signalled")
		return nil
	}
	err = stopCollector()
	for _, want := range []string{
		strconv.Itoa(pid),
		"TestUnrelatedMonitorListenerFixture",
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want live monitor owner detail %q", err, want)
		}
	}
	if signalErr := syscall.Kill(pid, 0); signalErr != nil {
		t.Fatalf("unrelated monitor fixture was stopped: %v", signalErr)
	}
}

func TestUnrelatedMonitorListenerFixture(t *testing.T) {
	if os.Getenv("GO_WANT_UNRELATED_MONITOR_FIXTURE") != "1" {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		os.Getenv("UNRELATED_MONITOR_READY"), []byte(port+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestStopCollectorStopsUnhealthyOwnedProcessWithMissingMetadata(t *testing.T) {
	restoreObservabilityHooks(t)
	t.Chdir(t.TempDir())
	locateCollectorProcess = func() (collectorProcess, bool, error) {
		return collectorFixtureProcess(4242, "/removed/worktree"), false, nil
	}
	requestCollectorExit = func() error { return errors.New("query is unhealthy") }
	signalled := 0
	terminateCollectorProcess = func(pid int) error {
		signalled = pid
		return nil
	}

	if err := stopCollector(); err != nil {
		t.Fatal(err)
	}
	if signalled != 4242 {
		t.Fatalf("signalled pid = %d, want 4242", signalled)
	}
}

func TestStopCollectorDoesNotTouchUnrelatedListener(t *testing.T) {
	restoreObservabilityHooks(t)
	t.Chdir(t.TempDir())
	locateCollectorProcess = func() (collectorProcess, bool, error) {
		return collectorProcess{}, false,
			errors.New("pid 4343 is not an owned collector: unrelated_server.py")
	}
	requestCollectorExit = func() error {
		t.Fatal("unrelated owner must not receive lifecycle request")
		return nil
	}
	terminateCollectorProcess = func(int) error {
		t.Fatal("unrelated owner must not be signalled")
		return nil
	}
	err := stopCollector()
	if err == nil || !strings.Contains(err.Error(), "unrelated_server.py") {
		t.Fatalf("error = %v, want unrelated owner", err)
	}
}

func TestTerminateCollectorFallsBackToVerifiedPIDWhenGroupSignalIsDenied(t *testing.T) {
	restoreObservabilityHooks(t)
	process := collectorFixtureProcess(4242, "/removed/worktree")
	collectorCommandOutput = func(name string, args ...string) (string, error) {
		return process.Command, nil
	}
	alive := true
	collectorProcessAlive = func(int) bool { return alive }
	var signalled []int
	collectorSignalProcess = func(pid int, signal syscall.Signal) error {
		signalled = append(signalled, pid)
		if pid < 0 {
			return syscall.EPERM
		}
		alive = false
		return nil
	}
	if err := terminateCollectorProcessGroup(process.PID); err != nil {
		t.Fatal(err)
	}
	if len(signalled) != 2 || signalled[0] != -process.PID || signalled[1] != process.PID {
		t.Fatalf("signals = %v, want group then verified pid", signalled)
	}
}

func TestObservabilityStatusReportsOwnerCommandAndRemovedSource(t *testing.T) {
	restoreObservabilityHooks(t)
	process := collectorFixtureProcess(4242, "/removed/worktree")
	locateCollectorProcess = func() (collectorProcess, bool, error) {
		return process, false, nil
	}
	currentCollectorSource = func() (collectorSource, error) {
		return collectorSource{
			Profile:   "/current/catalog/" + collectorProfileRel,
			Directory: "/current/catalog",
			CoreRoot:  "/current/agent-core",
		}, nil
	}
	checkObservability = func() error { return errors.New("query returned HTTP 500") }
	var output bytes.Buffer
	observabilityOutput = &output

	err := (Observability{}).Status()
	if err == nil {
		t.Fatal("unhealthy status succeeded")
	}
	for _, want := range []string{
		"pid 4242", process.Command, "source revision mismatch",
		"source was removed", "HTTP 500",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("status output %q does not contain %q", output.String(), want)
		}
	}
}

func TestObservabilityUpChecksPortsAndStarts(t *testing.T) {
	restoreObservabilityHooks(t)
	healthCalls := 0
	checkObservability = func() error {
		healthCalls++
		if healthCalls == 1 {
			return errors.New("not started")
		}
		return nil
	}
	var checked []string
	checkObservabilityPort = func(name, port string) error {
		checked = append(checked, name+":"+port)
		return nil
	}
	started := false
	startCollectorProcess = func() error {
		started = true
		return nil
	}

	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
	if len(checked) != 4 { // OTLP gRPC, control, monitor, query
		t.Fatalf("checked ports = %v", checked)
	}
	if !started {
		t.Fatal("collector process was not started")
	}
}

func TestObservabilityUpReportsPortCollisionBeforeStart(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return errors.New("not started") }
	checkObservabilityPort = func(name, port string) error {
		return errors.New("port owner")
	}
	startCollectorProcess = func() error {
		t.Fatal("collector must not start after a port collision")
		return nil
	}

	if err := (Observability{}).Up(); err == nil || !strings.Contains(err.Error(), "port owner") {
		t.Fatalf("error = %v", err)
	}
}

func TestObservabilityUpReplacesUnhealthyOwnedProcessBeforeStart(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return errors.New("not healthy yet") }
	locateCollectorProcess = func() (collectorProcess, bool, error) {
		return collectorFixtureProcess(4242, "/removed/worktree"), false, nil
	}
	var actions []string
	stopCollectorProcess = func() error {
		actions = append(actions, "stop")
		return nil
	}
	checkObservabilityPort = func(name, port string) error {
		actions = append(actions, "check "+port)
		return nil
	}
	startCollectorProcess = func() error {
		actions = append(actions, "start")
		checkObservability = func() error { return nil }
		return nil
	}

	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 6 || actions[0] != "stop" || actions[5] != "start" {
		t.Fatalf("actions = %v, want stop, four port checks, start", actions)
	}
}

func TestObservabilityDownStopsCollector(t *testing.T) {
	restoreObservabilityHooks(t)
	stopped := false
	stopCollectorProcess = func() error {
		stopped = true
		return nil
	}
	if err := (Observability{}).Down(); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("down did not stop the collector")
	}
}

func TestObservabilityPortsAreConfigurable(t *testing.T) {
	settings := observabilitySettingsFrom(demoConfig{
		OTELGRPCPort:         "24317",
		CollectorMonitorPort: "28192",
		CollectorQueryPort:   "28193",
	})
	ports := observabilityPortsFrom(settings)
	if ports[0].value != "24317" ||
		ports[2].value != "28192" ||
		ports[3].value != "28193" {
		t.Fatalf("ports = %#v", ports)
	}
	if ports[1].value != collectorControlPortDefault {
		t.Fatalf("unset control port = %s, want the %s default", ports[1].value, collectorControlPortDefault)
	}
}

func TestConcurrentObservabilityUpStartsOnceAndReusesSourceMatch(t *testing.T) {
	restoreObservabilityHooks(t)
	runCollectorReconciliation = withCollectorReconciliationLock
	lockPath := filepath.Join(t.TempDir(), "collector.lock")
	collectorReconciliationPath = func() (string, error) {
		return lockPath, nil
	}
	configuredObservabilityPorts = func() []namedPort { return nil }

	var mutex sync.Mutex
	healthy := false
	starts := 0
	sourceMatches := 0
	checkObservability = func() error {
		mutex.Lock()
		defer mutex.Unlock()
		if healthy {
			return nil
		}
		return errors.New("not started")
	}
	startCollectorProcess = func() error {
		mutex.Lock()
		starts++
		mutex.Unlock()
		time.Sleep(50 * time.Millisecond)
		mutex.Lock()
		healthy = true
		mutex.Unlock()
		return nil
	}
	readCollectorFingerprint = func() (string, error) {
		mutex.Lock()
		sourceMatches++
		mutex.Unlock()
		return "current", nil
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			results <- (Observability{}).Up()
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	mutex.Lock()
	defer mutex.Unlock()
	if starts != 1 || sourceMatches != 1 {
		t.Fatalf("concurrent up starts/source-matched reuses = %d/%d, want 1/1",
			starts, sourceMatches)
	}
}

func TestCollectorReconciliationLockRecoversAfterHolderCrash(t *testing.T) {
	restoreObservabilityHooks(t)
	root := t.TempDir()
	path := filepath.Join(root, "collector.lock")
	ready := filepath.Join(root, "ready")
	command := exec.Command(
		os.Args[0], "-test.run=^TestCollectorReconciliationLockFixture$")
	command.Env = append(os.Environ(),
		"GO_WANT_COLLECTOR_LOCK_FIXTURE=1",
		"COLLECTOR_LOCK_PATH="+path,
		"COLLECTOR_LOCK_READY="+ready,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatal("lock holder did not become ready")
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("killed lock holder exited successfully")
	}
	command.Process = nil

	lock, err := acquireCollectorLifecycleLock(path, "recovery", time.Second)
	if err != nil {
		t.Fatalf("acquire after crashed holder: %v", err)
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorReconciliationLockWaitIsBoundedAndNamesHolder(t *testing.T) {
	restoreObservabilityHooks(t)
	path := filepath.Join(t.TempDir(), "collector.lock")
	holder, err := acquireCollectorLifecycleLock(path, "holder", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.release() }()

	started := time.Now()
	_, err = acquireCollectorLifecycleLock(path, "contender", 20*time.Millisecond)
	if err == nil {
		t.Fatal("contender acquired a held collector lock")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded collector lock wait took %s", elapsed)
	}
	for _, want := range []string{"timed out", "action=holder", "reconciliation contender"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("lock error %q does not contain %q", err, want)
		}
	}
}

func TestCollectorReconciliationLockFixture(t *testing.T) {
	if os.Getenv("GO_WANT_COLLECTOR_LOCK_FIXTURE") != "1" {
		return
	}
	lock, err := acquireCollectorLifecycleLock(
		os.Getenv("COLLECTOR_LOCK_PATH"), "fixture", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.release() }()
	if err := os.WriteFile(
		os.Getenv("COLLECTOR_LOCK_READY"), []byte("ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestProcessAliveTreatsZombieAsExited(t *testing.T) {
	restoreObservabilityHooks(t)
	readCollectorProcessState = func(int) (string, error) {
		return "Z+", nil
	}
	if processAlive(os.Getpid()) {
		t.Fatal("zombie process state was treated as alive")
	}
}

func TestAggregateSessionStopsSharedObservabilityAndPreservesStandaloneLifetime(t *testing.T) {
	restoreObservabilityHooks(t)
	stops := 0
	stopCollectorProcess = func() error {
		stops++
		return nil
	}
	retainAggregateObservability()
	if stops != 0 {
		t.Fatal("standalone observability registered an aggregate teardown")
	}

	session := newIntegrationKindSession(t.TempDir())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	session.poison(errors.New("lane failed before aggregate completion"))
	if stops != 0 {
		t.Fatal("failure cleanup stopped observability before concurrent lanes completed")
	}
	retainAggregateObservability()
	retainAggregateObservability()
	session.close()
	if stops != 1 {
		t.Fatalf("aggregate observability stops = %d, want one", stops)
	}
}

func TestSharedAggregateReportsObservabilityTeardownFailure(t *testing.T) {
	restoreObservabilityHooks(t)
	stopErr := errors.New("collector remained alive")
	stopCollectorProcess = func() error { return stopErr }
	err := runSharedKindTargets(nil)
	if !errors.Is(err, stopErr) {
		t.Fatalf("aggregate error = %v, want %v", err, stopErr)
	}
}

func restoreObservabilityHooks(t *testing.T) {
	t.Helper()
	start := startCollectorProcess
	stop := stopCollectorProcess
	health := checkObservability
	port := checkObservabilityPort
	ports := configuredObservabilityPorts
	current := currentCollectorFingerprint
	read := readCollectorFingerprint
	write := writeCollectorFingerprint
	locate := locateCollectorProcess
	request := requestCollectorExit
	alive := collectorProcessAlive
	terminate := terminateCollectorProcess
	output := collectorCommandOutput
	signal := collectorSignalProcess
	stopWait := untrackedCollectorStopWait
	source := currentCollectorSource
	writer := observabilityOutput
	reconcile := runCollectorReconciliation
	lockPath := collectorReconciliationPath
	lockSleep := collectorReconciliationSleep
	processState := readCollectorProcessState
	currentCollectorFingerprint = func() (string, error) { return "current", nil }
	readCollectorFingerprint = func() (string, error) { return "current", nil }
	writeCollectorFingerprint = func(string) error { return nil }
	configuredObservabilityPorts = observabilityPorts
	locateCollectorProcess = func() (collectorProcess, bool, error) {
		return collectorProcess{}, false, errCollectorNotFound
	}
	currentCollectorSource = func() (collectorSource, error) {
		return collectorSource{}, nil
	}
	observabilityOutput = os.Stdout
	runCollectorReconciliation = func(_ string, run func() error) error {
		return run()
	}
	t.Cleanup(func() {
		startCollectorProcess = start
		stopCollectorProcess = stop
		checkObservability = health
		checkObservabilityPort = port
		configuredObservabilityPorts = ports
		currentCollectorFingerprint = current
		readCollectorFingerprint = read
		writeCollectorFingerprint = write
		locateCollectorProcess = locate
		requestCollectorExit = request
		collectorProcessAlive = alive
		terminateCollectorProcess = terminate
		collectorCommandOutput = output
		collectorSignalProcess = signal
		untrackedCollectorStopWait = stopWait
		currentCollectorSource = source
		observabilityOutput = writer
		runCollectorReconciliation = reconcile
		collectorReconciliationPath = lockPath
		collectorReconciliationSleep = lockSleep
		readCollectorProcessState = processState
	})
}

func collectorFixtureProcess(pid int, root string) collectorProcess {
	directory := filepath.Join(root, "applications", "catalog")
	profile := filepath.Join(directory, filepath.FromSlash(collectorProfileRel))
	coreRoot := filepath.Join(root, "agent-core")
	command := strings.Join([]string{
		"/tmp/chatbot-mesh-application-agent",
		"--profile", profile,
		"--directory", directory,
		"--core-root", coreRoot,
	}, " ")
	return collectorProcess{
		PID: pid, Command: command, Profile: profile,
		Directory: directory, CoreRoot: coreRoot,
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
