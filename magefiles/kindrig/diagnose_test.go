// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveDiagnoseTarget(t *testing.T) {
	cases := []struct {
		name        string
		scenario    string
		demoCluster string
		want        DiagnoseTarget
	}{
		{
			name: "scenario namespace on the shared platform", scenario: "helm-smoke",
			demoCluster: "da-chatbot-mesh-demo",
			want:        DiagnoseTarget{Cluster: PlatformClusterName, Namespace: "da-helm-smoke"},
		},
		{
			name: "demo cluster in the default namespace", scenario: DiagnoseDemoScenario,
			demoCluster: "da-coding-agent-demo",
			want:        DiagnoseTarget{Cluster: "da-coding-agent-demo", Namespace: "default"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveDiagnoseTarget(tc.scenario, tc.demoCluster)
			if err != nil || got != tc.want {
				t.Fatalf("ResolveDiagnoseTarget(%q, %q) = %+v, %v; want %+v",
					tc.scenario, tc.demoCluster, got, err, tc.want)
			}
		})
	}
}

func TestResolveDiagnoseTargetRejectsUnusableArguments(t *testing.T) {
	if _, err := ResolveDiagnoseTarget(DiagnoseDemoScenario, ""); err == nil {
		t.Error("an application without a demo cluster accepted the demo scenario")
	}
	for _, scenario := range []string{"", "Helm_Smoke", strings.Repeat("a", 62)} {
		if _, err := ResolveDiagnoseTarget(scenario, "da-demo"); err == nil {
			t.Errorf("ResolveDiagnoseTarget accepted invalid scenario %q", scenario)
		}
	}
}

func TestDiagnoseEvidenceDirectory(t *testing.T) {
	now := time.Date(2026, 9, 19, 16, 30, 5, 0, time.UTC)
	got := DiagnoseEvidenceDirectory("/app", "helm-smoke", now)
	want := filepath.Join("/app", "build", "kind-evidence", "diagnose-helm-smoke-20260919T163005Z")
	if got != want {
		t.Errorf("evidence directory = %s, want %s", got, want)
	}
}

func TestDiagnoseRefusesWhenTheClusterIsNotRunning(t *testing.T) {
	err := Diagnose(DiagnoseRequest{
		Scenario: DiagnoseDemoScenario, DemoCluster: "da-absent-cluster",
		ApplicationRoot: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("Diagnose error = %v, want a not-running cluster", err)
	}
}

func TestBoundedRunnerBoundsEveryCommand(t *testing.T) {
	var deadlines int
	run := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		if _, ok := ctx.Deadline(); ok {
			deadlines++
		}
		return nil, nil
	}
	bounded := boundedRunner(run, time.Second)
	for range 3 {
		if _, err := bounded("kubectl", "get", "pods"); err != nil {
			t.Fatal(err)
		}
	}
	if deadlines != 3 {
		t.Errorf("%d of 3 commands carried a deadline", deadlines)
	}
}

func TestDiagnoseAgentReportsWhenNothingResolved(t *testing.T) {
	err := DiagnoseAgent{}.diagnose(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "skipping diagnosis") {
		t.Fatalf("zero agent error = %v, want a skip report", err)
	}
}

// A rig doctor that exits non-zero has still reached a terminal state, so the
// diagnosis it wrote is reported rather than discarded (srd023 R4).
func TestDiagnoseAgentAcceptsAFailureTerminalThatWroteADiagnosis(t *testing.T) {
	dir := t.TempDir()
	agent := DiagnoseAgent{
		Binary:  "/bin/sh",
		Profile: filepath.Join(dir, "profile.yaml"),
	}
	if err := writeDiagnostic(filepath.Join(dir, diagnosisFile), []byte("probable_cause: x\n"), nil); err != nil {
		t.Fatal(err)
	}
	if err := agent.diagnose(dir); err != nil {
		t.Errorf("diagnose = %v, want the written diagnosis accepted", err)
	}
}

func TestDiagnoseAgentReportsAMissingDiagnosis(t *testing.T) {
	dir := t.TempDir()
	agent := DiagnoseAgent{Binary: "/bin/sh", Profile: filepath.Join(dir, "profile.yaml")}
	err := agent.diagnose(dir)
	if err == nil || !strings.Contains(err.Error(), "no diagnosis") {
		t.Fatalf("diagnose = %v, want a missing-diagnosis report", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Error("a missing diagnosis must not surface as a run failure")
	}
}
