// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const processHelperEnv = "CONFORMANCE_PROCESS_HELPER"

func TestManagedProcessHelper(t *testing.T) {
	mode := os.Getenv(processHelperEnv)
	if mode == "" {
		return
	}
	ready := os.Getenv("CONFORMANCE_PROCESS_READY")
	switch mode {
	case "cooperative":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		defer signal.Stop(signals)
		writeProcessHelperFile(ready, "ready")
		<-signals
		writeProcessHelperFile(os.Getenv("CONFORMANCE_PROCESS_TRACE"), processHelperTrace)
	case "ignore":
		signal.Ignore(syscall.SIGTERM)
		child := exec.Command("sleep", "30")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		writeProcessHelperFile(os.Getenv("CONFORMANCE_PROCESS_CHILD"), strconv.Itoa(child.Process.Pid))
		writeProcessHelperFile(ready, "ready")
		time.Sleep(30 * time.Second)
	default:
		os.Exit(2)
	}
}

func TestManagedProcessTerminatesCooperativelyWithTraceEvidence(t *testing.T) {
	started := time.Now()
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace.json")
	process := startProcessHelper(t, "cooperative", dir, tracePath)

	outcome := process.terminate(2 * time.Second)

	if outcome.Err != nil || outcome.Forced {
		t.Fatalf("cooperative termination = %+v", outcome)
	}
	if err := process.err(); err != nil {
		t.Fatalf("cooperative process wait: %v", err)
	}
	spans, err := ParseSpansFile(tracePath)
	if err != nil {
		t.Fatalf("parse cooperative trace: %v", err)
	}
	if root, ok := spans.Root(); !ok || root.Name != RootSpanName {
		t.Fatalf("cooperative trace root = %#v, found=%t", root, ok)
	}
	if evidence := timeoutTraceEvidence(tracePath); !strings.Contains(evidence, "last_stage=init.registry_frozen") {
		t.Fatalf("timeout evidence = %q", evidence)
	}
	if elapsed := time.Since(started); elapsed >= 4*time.Second {
		t.Fatalf("cooperative process test took %s", elapsed)
	}
}

func TestManagedProcessForceKillsLeaderAndGrandchild(t *testing.T) {
	started := time.Now()
	dir := t.TempDir()
	process := startProcessHelper(t, "ignore", dir, "")
	childPID := readProcessHelperPID(t, filepath.Join(dir, "child.pid"))
	leaderPID := process.cmd.Process.Pid

	outcome := process.terminate(50 * time.Millisecond)

	if outcome.Err != nil || !outcome.Forced {
		t.Fatalf("forced termination = %+v", outcome)
	}
	if err := process.err(); err == nil {
		t.Fatal("forced process exited without a signal error")
	}
	requireProcessGone(t, leaderPID)
	requireProcessGone(t, childPID)
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("forced process test took %s", elapsed)
	}
}

func startProcessHelper(t *testing.T, mode, dir, tracePath string) *managedProcess {
	t.Helper()
	ready := filepath.Join(dir, "ready")
	var output bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run=^TestManagedProcessHelper$")
	cmd.Env = append(os.Environ(),
		processHelperEnv+"="+mode,
		"CONFORMANCE_PROCESS_READY="+ready,
		"CONFORMANCE_PROCESS_TRACE="+tracePath,
		"CONFORMANCE_PROCESS_CHILD="+filepath.Join(dir, "child.pid"),
	)
	cmd.Stdout = &output
	cmd.Stderr = &output
	process, err := startManagedProcess(cmd)
	if err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitProcessHelperFile(t, ready, &output)
	return process
}

func waitProcessHelperFile(t *testing.T, path string, output *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper did not create %s\noutput:\n%s", path, output.String())
}

func readProcessHelperPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func requireProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d still exists", pid)
}

func writeProcessHelperFile(path, content string) {
	if path == "" {
		os.Exit(2)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		os.Exit(2)
	}
}

const processHelperTrace = `{
  "Name": "agent.run",
  "SpanContext": {
    "TraceID": "4bf92f3577b34da6a3ce929d0e0e4736",
    "SpanID": "00f067aa0ba902b7"
  },
  "Parent": {
    "TraceID": "00000000000000000000000000000000",
    "SpanID": "0000000000000000"
  },
  "Events": [{"Name": "init.registry_frozen"}],
  "Status": {"Code": "Unset"}
}`
