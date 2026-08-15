// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

type recordingCatalogRunner struct {
	mu       sync.Mutex
	calls    []catalogCommand
	failCall int
	failErr  error
	stdout   string
	stderr   string
}

func (recorder *recordingCatalogRunner) run(invocation catalogCommand) error {
	recorder.mu.Lock()
	invocation.args = append([]string(nil), invocation.args...)
	recorder.calls = append(recorder.calls, invocation)
	call := len(recorder.calls)
	failCall, failErr := recorder.failCall, recorder.failErr
	stdout, stderr := recorder.stdout, recorder.stderr
	recorder.mu.Unlock()

	if stdout != "" {
		_, _ = io.WriteString(invocation.stdout, stdout)
	}
	if stderr != "" {
		_, _ = io.WriteString(invocation.stderr, stderr)
	}
	if call == failCall {
		return failErr
	}
	return nil
}

func (recorder *recordingCatalogRunner) recordedCalls() []catalogCommand {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]catalogCommand(nil), recorder.calls...)
}

func newTestCatalogRunner(t *testing.T, recorder *recordingCatalogRunner) catalogTestRunner {
	t.Helper()
	return catalogTestRunner{
		catalogRoot: t.TempDir(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		runCommand:  recorder.run,
		mkdirTemp:   os.MkdirTemp,
		removeAll:   os.RemoveAll,
	}
}

func TestRunConformanceReportsCompileFailureAndCleansTempPath(t *testing.T) {
	wantErr := errors.New("compile failed")
	recorder := &recordingCatalogRunner{
		failCall: 1,
		failErr:  wantErr,
		stdout:   "compile stdout\n",
		stderr:   "compile stderr\n",
	}
	runner := newTestCatalogRunner(t, recorder)

	err := runner.runConformance(conformanceOptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runConformance error = %v, want compile error", err)
	}
	for _, want := range []string{"compile stdout", "compile stderr"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want captured %q", err, want)
		}
	}
	calls := recorder.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("commands = %d, want compile only", len(calls))
	}
	assertRemoved(t, filepath.Dir(outputPath(t, calls[0])))
}

func TestRunConformanceReportsExecutionFailureAndCleansTempPath(t *testing.T) {
	wantErr := errors.New("execution failed")
	recorder := &recordingCatalogRunner{
		failCall: 2,
		failErr:  wantErr,
		stdout:   "test stdout\n",
		stderr:   "test stderr\n",
	}
	runner := newTestCatalogRunner(t, recorder)

	err := runner.runConformance(conformanceOptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runConformance error = %v, want execution error", err)
	}
	calls := recorder.recordedCalls()
	if len(calls) != 2 {
		t.Fatalf("commands = %d, want compile and execution", len(calls))
	}
	assertRemoved(t, filepath.Dir(outputPath(t, calls[0])))
}

func TestRunConformanceUsesStableBinaryFlagsAndWorkingDirectory(t *testing.T) {
	recorder := &recordingCatalogRunner{}
	runner := newTestCatalogRunner(t, recorder)

	if err := runner.runConformance(conformanceOptions{
		live:        true,
		liveTimeout: "7m",
	}); err != nil {
		t.Fatalf("runConformance returned error: %v", err)
	}

	calls := recorder.recordedCalls()
	if len(calls) != 2 {
		t.Fatalf("commands = %d, want 2", len(calls))
	}
	binary := outputPath(t, calls[0])
	if calls[0].name != "go" {
		t.Errorf("compile command = %q, want go", calls[0].name)
	}
	wantCompileArgs := []string{"test", "-c", "-o", binary, "./conformance"}
	if !reflect.DeepEqual(calls[0].args, wantCompileArgs) {
		t.Errorf("compile args = %#v, want %#v", calls[0].args, wantCompileArgs)
	}
	if calls[0].dir != runner.catalogRoot {
		t.Errorf("compile cwd = %q, want %q", calls[0].dir, runner.catalogRoot)
	}
	if calls[1].name != binary {
		t.Errorf("execution command = %q, want %q", calls[1].name, binary)
	}
	wantExecutionArgs := []string{
		"-test.timeout=" + conformanceTestTimeout,
		"-test.count=1",
		"-live=true",
		"-live-timeout=7m",
	}
	if !reflect.DeepEqual(calls[1].args, wantExecutionArgs) {
		t.Errorf("execution args = %#v, want %#v", calls[1].args, wantExecutionArgs)
	}
	wantCWD := filepath.Join(runner.catalogRoot, "conformance")
	if calls[1].dir != wantCWD {
		t.Errorf("execution cwd = %q, want %q", calls[1].dir, wantCWD)
	}
	assertRemoved(t, filepath.Dir(binary))
}

func TestRunConformanceDisablesLiveByDefault(t *testing.T) {
	recorder := &recordingCatalogRunner{}
	runner := newTestCatalogRunner(t, recorder)

	if err := runner.runConformance(conformanceOptions{}); err != nil {
		t.Fatalf("runConformance returned error: %v", err)
	}
	args := recorder.recordedCalls()[1].args
	want := []string{
		"-test.timeout=" + conformanceTestTimeout,
		"-test.count=1",
		"-live=false",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("execution args = %#v, want %#v", args, want)
	}
}

func TestRunNonConformanceUsesNormalGoTestAndExcludesConformance(t *testing.T) {
	recorder := &recordingCatalogRunner{}
	runner := newTestCatalogRunner(t, recorder)
	recorder.stdout = strings.Join([]string{
		"example.test/agentbuild",
		"example.test/catalogroot",
		"example.test/conformance",
		"example.test/magefiles",
		"",
	}, "\n")

	if err := runner.runNonConformance(); err != nil {
		t.Fatalf("runNonConformance returned error: %v", err)
	}
	calls := recorder.recordedCalls()
	if len(calls) != 2 {
		t.Fatalf("commands = %d, want go list and go test", len(calls))
	}
	wantArgs := []string{
		"test",
		"example.test/agentbuild",
		"example.test/catalogroot",
		"example.test/magefiles",
	}
	if calls[1].name != "go" || !reflect.DeepEqual(calls[1].args, wantArgs) {
		t.Fatalf("test command = %s %#v, want go %#v", calls[1].name, calls[1].args, wantArgs)
	}
}

func TestCatalogTestModeDoesNotRepeatReleaseConformance(t *testing.T) {
	recorder := &recordingCatalogRunner{stdout: "example.test/catalogroot\n"}
	runner := newTestCatalogRunner(t, recorder)

	if err := runner.runMode(catalogTestMode{nonConformance: true}); err != nil {
		t.Fatalf("catalog test mode returned error: %v", err)
	}
	calls := recorder.recordedCalls()
	if len(calls) != 2 {
		t.Fatalf("catalog test commands = %d, want go list and go test only", len(calls))
	}
	for _, call := range calls {
		if call.name != "go" {
			t.Fatalf("catalog test unexpectedly executed conformance binary %q", call.name)
		}
		if slices.Contains(call.args, "-c") {
			t.Fatalf("catalog test unexpectedly compiled conformance: %#v", call.args)
		}
	}
}

func TestRunConformanceUsesUniquePathsInParallel(t *testing.T) {
	const runs = 16
	recorder := &recordingCatalogRunner{}
	runner := newTestCatalogRunner(t, recorder)

	errs := make(chan error, runs)
	var group sync.WaitGroup
	for range runs {
		group.Add(1)
		go func() {
			defer group.Done()
			errs <- runner.runConformance(conformanceOptions{})
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("parallel runConformance returned error: %v", err)
		}
	}

	paths := make(map[string]bool, runs)
	for _, call := range recorder.recordedCalls() {
		if call.name != "go" {
			continue
		}
		path := outputPath(t, call)
		if paths[path] {
			t.Fatalf("reused conformance binary path %q", path)
		}
		paths[path] = true
		assertRemoved(t, filepath.Dir(path))
	}
	if len(paths) != runs {
		t.Fatalf("unique compile paths = %d, want %d", len(paths), runs)
	}
}

func TestRunConformanceForwardsPhaseOutput(t *testing.T) {
	recorder := &recordingCatalogRunner{
		stdout: "child stdout\n",
		stderr: "child stderr\n",
	}
	var stdout, stderr bytes.Buffer
	runner := newTestCatalogRunner(t, recorder)
	runner.stdout = &stdout
	runner.stderr = &stderr

	if err := runner.runConformance(conformanceOptions{}); err != nil {
		t.Fatalf("runConformance returned error: %v", err)
	}
	for _, phase := range []string{"conformance compile", "conformance execution"} {
		if !strings.Contains(stdout.String(), phase) {
			t.Errorf("stdout = %q, want phase %q", stdout.String(), phase)
		}
	}
	if got := strings.Count(stdout.String(), "child stdout"); got != 2 {
		t.Errorf("forwarded stdout count = %d, want 2", got)
	}
	if got := strings.Count(stderr.String(), "child stderr"); got != 2 {
		t.Errorf("forwarded stderr count = %d, want 2", got)
	}
}

func outputPath(t *testing.T, invocation catalogCommand) string {
	t.Helper()
	for index, arg := range invocation.args {
		if arg == "-o" && index+1 < len(invocation.args) {
			return invocation.args[index+1]
		}
	}
	t.Fatalf("command %s %#v has no -o output", invocation.name, invocation.args)
	return ""
}

func assertRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary path %s still exists: %v", path, err)
	}
}

func TestRunConformanceReportsCleanupFailure(t *testing.T) {
	wantErr := errors.New("cleanup failed")
	recorder := &recordingCatalogRunner{}
	runner := newTestCatalogRunner(t, recorder)
	var tempDir string
	runner.mkdirTemp = func(dir, pattern string) (string, error) {
		var err error
		tempDir, err = os.MkdirTemp(dir, pattern)
		return tempDir, err
	}
	runner.removeAll = func(string) error { return wantErr }
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	err := runner.runConformance(conformanceOptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runConformance error = %v, want cleanup error", err)
	}
}
