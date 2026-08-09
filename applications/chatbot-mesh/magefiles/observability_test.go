// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestDiscoverUntrackedCollectorRequiresOneVerifiedOwner(t *testing.T) {
	restoreObservabilityHooks(t)
	collectorCommandOutput = func(name string, args ...string) (string, error) {
		if name == "lsof" {
			return "4242\n", nil
		}
		return "/tmp/agent --profile /repo/applications/catalog/agents/collector/profile.yaml\n", nil
	}
	pid, err := discoverUntrackedCollector()
	if err != nil {
		t.Fatal(err)
	}
	if pid != 4242 {
		t.Fatalf("pid = %d, want 4242", pid)
	}
}

func TestDiscoverUntrackedCollectorRejectsDifferentOwners(t *testing.T) {
	restoreObservabilityHooks(t)
	collectorCommandOutput = func(name string, args ...string) (string, error) {
		if name != "lsof" {
			t.Fatal("process command must not be read after owner mismatch")
		}
		if strings.Contains(strings.Join(args, " "), ":"+collectorControlPortDefault) {
			return "4343\n", nil
		}
		return "4242\n", nil
	}
	_, err := discoverUntrackedCollector()
	if err == nil || !strings.Contains(err.Error(), "different owners") {
		t.Fatalf("error = %v, want different owners", err)
	}
}

func TestDiscoverUntrackedCollectorRejectsUnrelatedProcess(t *testing.T) {
	restoreObservabilityHooks(t)
	collectorCommandOutput = func(name string, args ...string) (string, error) {
		if name == "lsof" {
			return "4242\n", nil
		}
		return "/usr/bin/python unrelated_server.py\n", nil
	}
	_, err := discoverUntrackedCollector()
	if err == nil || !strings.Contains(err.Error(), "does not run the collector profile") {
		t.Fatalf("error = %v, want collector profile refusal", err)
	}
}

func TestStopCollectorSignalsVerifiedUntrackedProcessAfterTimeout(t *testing.T) {
	restoreObservabilityHooks(t)
	t.Chdir(t.TempDir())
	checkObservability = func() error { return nil }
	findUntrackedCollector = func() (int, error) { return 4242, nil }
	requestCollectorExit = func() error { return nil }
	collectorProcessAlive = func(int) bool { return true }
	untrackedCollectorStopWait = time.Millisecond
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

func TestStopCollectorDoesNotTouchAmbiguousUntrackedOwner(t *testing.T) {
	restoreObservabilityHooks(t)
	t.Chdir(t.TempDir())
	checkObservability = func() error { return nil }
	findUntrackedCollector = func() (int, error) {
		return 0, errors.New("different owners")
	}
	requestCollectorExit = func() error {
		t.Fatal("ambiguous owner must not receive lifecycle request")
		return nil
	}
	terminateCollectorProcess = func(int) error {
		t.Fatal("ambiguous owner must not be signalled")
		return nil
	}
	err := stopCollector()
	if err == nil || !strings.Contains(err.Error(), "different owners") {
		t.Fatalf("error = %v, want different owners", err)
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
	collectorAlreadyRunning = func() bool { return false }
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
	if len(checked) != 3 { // OTLP gRPC, Collector control, Collector query
		t.Fatalf("checked ports = %v", checked)
	}
	if !started {
		t.Fatal("collector process was not started")
	}
}

func TestObservabilityUpReportsPortCollisionBeforeStart(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return errors.New("not started") }
	collectorAlreadyRunning = func() bool { return false }
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

func TestObservabilityUpSkipsPortCheckWhenRunning(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return errors.New("not healthy yet") }
	collectorAlreadyRunning = func() bool { return true }
	checkObservabilityPort = func(name, port string) error {
		t.Fatal("a running ingress must not re-check ports")
		return nil
	}
	starts := 0
	startCollectorProcess = func() error {
		starts++
		checkObservability = func() error { return nil }
		return nil
	}

	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatalf("starts = %d, want 1", starts)
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
		OTELGRPCPort:       "24317",
		CollectorQueryPort: "28193",
	})
	ports := observabilityPortsFrom(settings)
	if ports[0].value != "24317" || ports[2].value != "28193" {
		t.Fatalf("ports = %#v", ports)
	}
	if ports[1].value != collectorControlPortDefault {
		t.Fatalf("unset control port = %s, want the %s default", ports[1].value, collectorControlPortDefault)
	}
}

func restoreObservabilityHooks(t *testing.T) {
	t.Helper()
	start := startCollectorProcess
	stop := stopCollectorProcess
	health := checkObservability
	port := checkObservabilityPort
	running := collectorAlreadyRunning
	current := currentCollectorFingerprint
	read := readCollectorFingerprint
	write := writeCollectorFingerprint
	find := findUntrackedCollector
	request := requestCollectorExit
	alive := collectorProcessAlive
	terminate := terminateCollectorProcess
	output := collectorCommandOutput
	stopWait := untrackedCollectorStopWait
	currentCollectorFingerprint = func() (string, error) { return "current", nil }
	readCollectorFingerprint = func() (string, error) { return "current", nil }
	writeCollectorFingerprint = func(string) error { return nil }
	t.Cleanup(func() {
		startCollectorProcess = start
		stopCollectorProcess = stop
		checkObservability = health
		checkObservabilityPort = port
		collectorAlreadyRunning = running
		currentCollectorFingerprint = current
		readCollectorFingerprint = read
		writeCollectorFingerprint = write
		findUntrackedCollector = find
		requestCollectorExit = request
		collectorProcessAlive = alive
		terminateCollectorProcess = terminate
		collectorCommandOutput = output
		untrackedCollectorStopWait = stopWait
	})
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
