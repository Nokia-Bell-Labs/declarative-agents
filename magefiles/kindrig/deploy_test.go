// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubAgent writes a shell script that exits with the given code, standing for
// an agent run that reached a success or a failure terminal.
func stubAgent(t *testing.T, exitCode int) DeployAgent {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "agent")
	script := "#!/bin/sh\nexit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return DeployAgent{Binary: path, Profile: filepath.Join(dir, "profile.yaml"), CoreRoot: dir}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

func helmVersion(version string) func() (string, error) {
	return func() (string, error) { return version, nil }
}

// A deploy against a cluster that is not running stops before any Helm word.
// For undeploy this is part of the machine's correctness contract: its probe
// reads a failing helm_history as an absent release, which is only safe once an
// unreachable cluster has been ruled out (srd022 R6.5).
func TestDeployRefusesWhenTheClusterIsNotRunning(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"deploy", "undeploy"} {
		request := DeployRequest{
			Cluster:     "kindrig-absent-cluster-" + verb,
			Coordinates: completeCoordinates(t),
			Agent:       stubAgent(t, 0),
			HelmVersion: helmVersion("v3.16.3"),
		}
		var err error
		if verb == "deploy" {
			err = Deploy(request)
		} else {
			err = Undeploy(request)
		}
		if err == nil || !strings.Contains(err.Error(), "is not running") {
			t.Errorf("%s against an absent cluster = %v, want a not-running error", verb, err)
		}
	}
}

// helm 4 accepts every flag the deploy words carry -- it deprecates --atomic
// and a bare --dry-run and warns -- so a mismatched major is reported and the
// run continues. Gating here would block a deploy that works.
func TestDeployWarnsButRunsOnAnotherHelmMajor(t *testing.T) {
	t.Parallel()
	warning := captureStdout(t, func() {
		if err := warnOnHelmMajor(helmVersion("v4.2.4+g3900f43")); err != nil {
			t.Errorf("helm 4 = %v, want the run to continue", err)
		}
	})
	for _, want := range []string{"major version 4", "--rollback-on-failure", "--dry-run=client"} {
		if !strings.Contains(warning, want) {
			t.Errorf("warning = %q, want it to mention %q", warning, want)
		}
	}

	quiet := captureStdout(t, func() {
		if err := warnOnHelmMajor(helmVersion("v3.16.3+gf5b8d2e")); err != nil {
			t.Errorf("helm 3 = %v, want acceptance", err)
		}
	})
	if quiet != "" {
		t.Errorf("helm 3 printed %q, want silence", quiet)
	}
}

// An unrecognizable version string still means helm answered, so the words will
// run and the operator only needs telling.
func TestDeployWarnsOnAnUnrecognizableHelmVersion(t *testing.T) {
	t.Parallel()
	warning := captureStdout(t, func() {
		if err := warnOnHelmMajor(helmVersion("not a version")); err != nil {
			t.Errorf("unrecognizable version = %v, want the run to continue", err)
		}
	})
	if !strings.Contains(warning, "not recognizable") {
		t.Errorf("warning = %q, want it to name the unreadable version", warning)
	}
}

// A probe that cannot execute is different: helm is missing or unusable, and
// every word is about to fail anyway.
func TestDeployFailsWhenTheHelmProbeCannotRun(t *testing.T) {
	t.Parallel()
	err := warnOnHelmMajor(func() (string, error) { return "", errors.New("exec: helm: not found") })
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("probe failure = %v, want the error surfaced", err)
	}
}

// captureStdout collects what a function prints, so the warning's wording is
// asserted rather than assumed.
func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	original := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	done := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, read)
		done <- buffer.String()
	}()
	run()
	_ = write.Close()
	os.Stdout = original
	return <-done
}

// Unlike a diagnosis, which reports and never gates, a deploy that did not come
// up fails the build.
func TestDeployGatesOnAFailureTerminal(t *testing.T) {
	t.Parallel()
	failing := stubAgent(t, 2)
	err := failing.run("deploy", filepath.Join(t.TempDir(), "profile.yaml"), t.TempDir(), t.TempDir(), []string{"Deployed"})
	if err == nil {
		t.Fatal("failure terminal = nil, want an error that fails the build")
	}
	if !strings.Contains(err.Error(), "failure terminal") {
		t.Errorf("error = %v, want it to name the failure terminal", err)
	}

	succeeding := stubAgent(t, 0)
	if err := succeeding.run("deploy", filepath.Join(t.TempDir(), "profile.yaml"), t.TempDir(), t.TempDir(), []string{"Deployed"}); err != nil {
		t.Errorf("success terminal = %v, want no error", err)
	}
}

// A deploy that cannot resolve its agent has produced nothing, so it errors
// rather than reporting a skip the way the rig doctor does.
func TestDeployRefusesWithoutAnAgent(t *testing.T) {
	t.Parallel()
	err := runDeployMachine(DeployRequest{Cluster: ""}, "deploy", "Deployed")
	if err == nil || !strings.Contains(err.Error(), "no cluster named") {
		t.Fatalf("empty cluster = %v, want a named-cluster error", err)
	}
}

func TestWriteDeployRequestCarriesPathAndContent(t *testing.T) {
	t.Parallel()
	destination := t.TempDir()
	overrides := "image:\n  tag: \"20260919\"\n"
	path, err := writeDeployRequest(destination, overrides)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var seed struct {
		Parameters struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(data, &seed); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if seed.Parameters.Path != overridesFileName {
		t.Errorf("path = %q, want %q", seed.Parameters.Path, overridesFileName)
	}
	if seed.Parameters.Content != overrides {
		t.Errorf("content = %q, want %q", seed.Parameters.Content, overrides)
	}
	// A numeric-looking tag has to survive as a string, or helm reads it as a
	// number and the image reference stops resolving.
	if !strings.Contains(seed.Parameters.Content, `"20260919"`) {
		t.Errorf("content = %q, want the tag to stay quoted", seed.Parameters.Content)
	}
}

// The render directory exists so a developer can read and re-run the exact
// command that ran. Pointing the words at kindrig's temporary kubeconfig, which
// is deleted when the run returns, left every rendered argv naming a file that
// no longer existed; re-running one failed with an unrelated cluster-unreachable
// error. The staged copy lives beside the declarations (GH-2304).
func TestStagedKubeconfigOutlivesTheRun(t *testing.T) {
	t.Parallel()
	destination := t.TempDir()
	source := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(source, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(destination, "kubeconfig")
	if err := os.WriteFile(staged, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The temporary original going away must not affect the staged copy.
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(staged)
	if err != nil {
		t.Fatalf("staged kubeconfig did not survive: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("staged kubeconfig mode = %o, want 600; it carries cluster credentials", mode)
	}
	coordinates := completeCoordinates(t)
	coordinates.Kubeconfig = staged
	rendered, err := RenderDeployDeclarations(shippedDeployDeclarations(t), coordinates, destination)
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range renderArgs(t, rendered) {
		if !containsArg(args, staged) {
			t.Errorf("%s argv = %v, want the staged kubeconfig %q", name, args, staged)
		}
	}
}
