// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"encoding/json"
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

func TestDeployRejectsAnUnsupportedHelmMajor(t *testing.T) {
	t.Parallel()
	err := checkHelmMajor(helmVersion("v4.0.1+gabc123"))
	if err == nil {
		t.Fatal("helm 4 = nil, want a rejection")
	}
	for _, want := range []string{"major version 4", "--atomic"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
	if err := checkHelmMajor(helmVersion("v3.16.3+gf5b8d2e")); err != nil {
		t.Errorf("helm 3 = %v, want acceptance", err)
	}
	if err := checkHelmMajor(helmVersion("not a version")); err == nil {
		t.Error("unrecognizable version = nil, want a rejection")
	}
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
