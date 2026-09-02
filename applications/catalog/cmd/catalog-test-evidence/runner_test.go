// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testModule = "example.test/catalog"

type recordingEvidenceCommands struct {
	calls    []evidenceCommand
	failCall int
	failErr  error
}

func (recorder *recordingEvidenceCommands) run(invocation evidenceCommand) error {
	invocation.args = append([]string(nil), invocation.args...)
	invocation.env = append([]string(nil), invocation.env...)
	recorder.calls = append(recorder.calls, invocation)
	switch {
	case reflect.DeepEqual(invocation.args, []string{"list", "-m"}):
		_, _ = io.WriteString(invocation.stdout, testModule+"\n")
	case reflect.DeepEqual(invocation.args, []string{"list", "./..."}):
		_, _ = io.WriteString(invocation.stdout, strings.Join([]string{
			testModule + "/agentbuild",
			testModule + "/catalogroot",
			testModule + "/cmd/catalog-test-evidence",
			testModule + "/conformance",
			testModule + "/magefiles",
			"",
		}, "\n"))
	}
	if len(recorder.calls) == recorder.failCall {
		return recorder.failErr
	}
	return nil
}

func TestMainModuleCommandSelectsCurrentWorkspaceModule(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go executable unavailable: %v", err)
	}
	if _, err := exec.LookPath("env"); err != nil {
		t.Skipf("env executable unavailable: %v", err)
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	sibling := filepath.Join(root, "sibling")
	for _, dir := range []string{target, sibling} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(target, "go.mod"), "module example.test/target\n\ngo 1.22\n")
	write(filepath.Join(sibling, "go.mod"), "module example.test/sibling\n\ngo 1.22\n")
	workspace := filepath.Join(root, "go.work")
	write(workspace, "go 1.22\n\nuse (\n\t./target\n\t./sibling\n)\n")
	moduleIdentityArgs := []string{"GOWORK=off", "go", "list", "-m"}

	for name, gowork := range map[string]string{
		"workspace active": workspace,
		"workspace off":    "off",
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("env", moduleIdentityArgs...)
			cmd.Dir = target
			cmd.Env = append(os.Environ(), "GOWORK="+gowork)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("env %s failed: %v\n%s", strings.Join(moduleIdentityArgs, " "), err, output)
			}
			if got, want := strings.TrimSpace(string(output)), "example.test/target"; got != want {
				t.Errorf("main module = %q, want %q", got, want)
			}
		})
	}
}

func newTestEvidenceRunner(t *testing.T, commands *recordingEvidenceCommands) evidenceRunner {
	t.Helper()
	return evidenceRunner{
		root:       t.TempDir(),
		stdout:     io.Discard,
		stderr:     io.Discard,
		runCommand: commands.run,
		mkdirTemp:  os.MkdirTemp,
		removeAll:  os.RemoveAll,
	}
}

func TestExecuteEvidenceCommandOverlaysPrivateTempDir(t *testing.T) {
	privateTemp := t.TempDir()
	var stdout bytes.Buffer

	err := executeEvidenceCommand(evidenceCommand{
		name:   "sh",
		args:   []string{"-c", `printf %s "$TMPDIR"`},
		env:    []string{"TMPDIR=" + privateTemp},
		stdout: &stdout,
	})

	if err != nil {
		t.Fatalf("execute evidence command: %v", err)
	}
	if got := stdout.String(); got != privateTemp {
		t.Fatalf("child TMPDIR = %q, want %q", got, privateTemp)
	}
}

func TestEvidenceRunnerInventoriesCompleteRosterWithStableConformance(t *testing.T) {
	commands := &recordingEvidenceCommands{}
	runner := newTestEvidenceRunner(t, commands)

	if err := runner.run([]string{"test", "-json", "-list", "^Test", "./..."}); err != nil {
		t.Fatalf("inventory returned error: %v", err)
	}
	if len(commands.calls) != 5 {
		t.Fatalf("commands = %d, want module, packages, normal tests, compile, execute", len(commands.calls))
	}
	wantNormal := []string{
		"test", "-json", "-list", "^Test",
		testModule + "/agentbuild",
		testModule + "/catalogroot",
		testModule + "/cmd/catalog-test-evidence",
		testModule + "/magefiles",
	}
	if !reflect.DeepEqual(commands.calls[2].args, wantNormal) {
		t.Errorf("normal inventory args = %#v, want %#v", commands.calls[2].args, wantNormal)
	}
	binary := compileOutput(t, commands.calls[3])
	wantStable := []string{
		"tool", "test2json", "-t", "-p", testModule + "/conformance", binary,
		"-test.v=true", "-test.timeout=" + testTimeout, "-test.list=^Test",
	}
	if !reflect.DeepEqual(commands.calls[4].args, wantStable) {
		t.Errorf("stable inventory args = %#v, want %#v", commands.calls[4].args, wantStable)
	}
	if commands.calls[4].dir != filepath.Join(runner.root, "conformance") {
		t.Errorf("stable inventory cwd = %q", commands.calls[4].dir)
	}
	assertStableTempEnv(t, commands.calls[3], binary)
	assertStableTempEnv(t, commands.calls[4], binary)
	assertEvidenceTempRemoved(t, binary)
}

func TestEvidenceRunnerExecutesCompleteRosterWithStableConformance(t *testing.T) {
	commands := &recordingEvidenceCommands{}
	runner := newTestEvidenceRunner(t, commands)

	if err := runner.run([]string{"test", "-json", "-count=1", "./..."}); err != nil {
		t.Fatalf("execution returned error: %v", err)
	}
	wantNormalPrefix := []string{"test", "-json", "-count=1"}
	if !reflect.DeepEqual(commands.calls[2].args[:3], wantNormalPrefix) {
		t.Errorf("normal execution args = %#v", commands.calls[2].args)
	}
	binary := compileOutput(t, commands.calls[3])
	wantStable := []string{
		"tool", "test2json", "-t", "-p", testModule + "/conformance", binary,
		"-test.v=true", "-test.timeout=" + testTimeout, "-test.count=1", "-live=false",
	}
	if !reflect.DeepEqual(commands.calls[4].args, wantStable) {
		t.Errorf("stable execution args = %#v, want %#v", commands.calls[4].args, wantStable)
	}
	assertStableTempEnv(t, commands.calls[3], binary)
	assertStableTempEnv(t, commands.calls[4], binary)
	assertEvidenceTempRemoved(t, binary)
}

func TestEvidenceRunnerCompileFailurePropagatesAndCleans(t *testing.T) {
	wantErr := errors.New("compile failed")
	commands := &recordingEvidenceCommands{failCall: 4, failErr: wantErr}
	runner := newTestEvidenceRunner(t, commands)

	err := runner.run([]string{"test", "-json", "-count=1", "./..."})
	if !errors.Is(err, wantErr) {
		t.Fatalf("execution error = %v, want compile failure", err)
	}
	if len(commands.calls) != 4 {
		t.Fatalf("commands = %d, want no conformance execution", len(commands.calls))
	}
	assertEvidenceTempRemoved(t, compileOutput(t, commands.calls[3]))
}

func TestEvidenceRunnerContinuesConformanceAfterNormalPackageFailure(t *testing.T) {
	wantErr := errors.New("normal tests failed")
	commands := &recordingEvidenceCommands{failCall: 3, failErr: wantErr}
	runner := newTestEvidenceRunner(t, commands)

	err := runner.run([]string{"test", "-json", "-count=1", "./..."})
	if !errors.Is(err, wantErr) {
		t.Fatalf("execution error = %v, want normal package failure", err)
	}
	if len(commands.calls) != 5 {
		t.Fatalf("commands = %d, want conformance compile and execution after normal failure", len(commands.calls))
	}
	assertEvidenceTempRemoved(t, compileOutput(t, commands.calls[3]))
}

func TestEvidenceRunnerExecutionFailurePropagatesAndCleans(t *testing.T) {
	wantErr := errors.New("execution failed")
	commands := &recordingEvidenceCommands{failCall: 5, failErr: wantErr}
	runner := newTestEvidenceRunner(t, commands)

	err := runner.run([]string{"test", "-json", "-count=1", "./..."})
	if !errors.Is(err, wantErr) {
		t.Fatalf("execution error = %v, want test failure", err)
	}
	assertEvidenceTempRemoved(t, compileOutput(t, commands.calls[3]))
}

func compileOutput(t *testing.T, invocation evidenceCommand) string {
	t.Helper()
	for index, arg := range invocation.args {
		if arg == "-o" && index+1 < len(invocation.args) {
			return invocation.args[index+1]
		}
	}
	t.Fatalf("compile command %#v has no output", invocation.args)
	return ""
}

func assertStableTempEnv(t *testing.T, invocation evidenceCommand, binary string) {
	t.Helper()
	want := []string{"TMPDIR=" + filepath.Dir(binary)}
	if !reflect.DeepEqual(invocation.env, want) {
		t.Errorf("stable conformance environment = %#v, want %#v", invocation.env, want)
	}
}

func assertEvidenceTempRemoved(t *testing.T, binary string) {
	t.Helper()
	if _, err := os.Stat(filepath.Dir(binary)); !os.IsNotExist(err) {
		t.Fatalf("temporary path for %s still exists: %v", binary, err)
	}
}
