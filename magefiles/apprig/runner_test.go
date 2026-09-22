// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

func runnerFixture(t *testing.T) Runner {
	t.Helper()
	root := t.TempDir()
	manifest := filepath.Join(root, "application.yaml")
	if err := os.WriteFile(manifest, []byte(`schema_version: 1
application: fixture
ownership: agent-owning
deployment:
  entries:
    - {id: fixture, workload: fixture, profile_path: agents/fixture/profile.yaml}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	agent := func() (kindrig.DeployAgent, error) {
		return kindrig.DeployAgent{Binary: "agent", Profile: "canonical", CoreRoot: root}, nil
	}
	return Runner{
		ManifestPath: manifest,
		Binding: PlatformBinding{
			Cluster: "da-platform", ApplicationRoot: root,
			ChartPath: "fixture-chart", ValuesPath: "fixture-values", Timeout: "2m",
		},
		CatalogRoot:   "catalog",
		DeployAgent:   agent,
		UndeployAgent: agent,
		DiagnoseAgent: func() (kindrig.DiagnoseAgent, error) { return kindrig.DiagnoseAgent{Binary: "agent"}, nil },
	}
}

// GH-2493 AC1/AC3: preparation precedes namespace and the canonical deploy
// operation, then application verification; invocation-owned preparation is
// cleaned after success.
func TestRunnerUpSequence(t *testing.T) {
	runner := runnerFixture(t)
	var sequence []string
	runner.Prepare = func(Resolved) (Preparation, error) {
		sequence = append(sequence, "prepare")
		return Preparation{
			ChartPath: "prepared-chart", ValuesPath: "prepared-values", Overrides: "fixture: true",
			Cleanup: func() error { sequence = append(sequence, "cleanup"); return nil },
		}, nil
	}
	runner.Verify = func(Resolved) error { sequence = append(sequence, "verify"); return nil }
	runner.operations = lifecycleOperations{
		ensureNamespace: func(request kindrig.ApplicationNamespaceRequest) (bool, error) {
			sequence = append(sequence, "namespace")
			if request.Namespace != "da-fixture" {
				t.Fatalf("namespace = %q", request.Namespace)
			}
			return true, nil
		},
		deploy: func(request kindrig.DeployRequest) error {
			sequence = append(sequence, "deploy")
			if request.Coordinates.ChartPath != "prepared-chart" ||
				request.Coordinates.ValuesPath != "prepared-values" ||
				request.Overrides != "fixture: true" {
				t.Fatalf("deploy request did not use preparation: %+v", request)
			}
			return nil
		},
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare", "namespace", "deploy", "verify", "cleanup"}
	if !reflect.DeepEqual(sequence, want) {
		t.Fatalf("sequence = %v, want %v", sequence, want)
	}
}

// GH-2493 AC3/AC8: an application verification failure compensates only after
// deployment, through undeploy before namespace cleanup.
func TestRunnerVerifyFailureCompensatesInOrder(t *testing.T) {
	runner := runnerFixture(t)
	var sequence []string
	runner.Verify = func(Resolved) error {
		sequence = append(sequence, "verify")
		return errors.New("injected verification failure")
	}
	runner.operations = lifecycleOperations{
		ensureNamespace: func(kindrig.ApplicationNamespaceRequest) (bool, error) {
			sequence = append(sequence, "namespace")
			return true, nil
		},
		deploy: func(kindrig.DeployRequest) error {
			sequence = append(sequence, "deploy")
			return nil
		},
		undeploy: func(kindrig.DeployRequest) error {
			sequence = append(sequence, "undeploy")
			return nil
		},
		deleteNamespace: func(kindrig.ApplicationNamespaceRequest) error {
			sequence = append(sequence, "delete-namespace")
			return nil
		},
	}
	err := runner.Up()
	if err == nil {
		t.Fatalf("verify failure did not fail Up: %v", err)
	}
	want := []string{"namespace", "deploy", "verify", "undeploy", "delete-namespace"}
	if !reflect.DeepEqual(sequence, want) {
		t.Fatalf("sequence = %v, want %v", sequence, want)
	}
}

// GH-2493 AC8: failed preparation cleans only its own artifacts and never
// creates a namespace or invokes a lifecycle agent.
func TestRunnerPreparationFailureStopsBeforeMutation(t *testing.T) {
	runner := runnerFixture(t)
	var sequence []string
	runner.Prepare = func(Resolved) (Preparation, error) {
		sequence = append(sequence, "prepare")
		return Preparation{Cleanup: func() error {
			sequence = append(sequence, "cleanup")
			return nil
		}}, errors.New("injected preparation failure")
	}
	runner.operations = lifecycleOperations{
		ensureNamespace: func(kindrig.ApplicationNamespaceRequest) (bool, error) {
			t.Fatal("namespace mutated after preparation failure")
			return false, nil
		},
		deploy: func(kindrig.DeployRequest) error {
			t.Fatal("deploy ran after preparation failure")
			return nil
		},
	}
	if err := runner.Up(); err == nil {
		t.Fatal("preparation failure was accepted")
	}
	want := []string{"prepare", "cleanup"}
	if !reflect.DeepEqual(sequence, want) {
		t.Fatalf("sequence = %v, want %v", sequence, want)
	}
}

// GH-2493 AC4/AC9: down reaches the canonical undeploy success before namespace
// cleanup, then reports the retained bucket and absent query independently.
func TestRunnerDownOrdersCleanupAndReportsRetention(t *testing.T) {
	runner := runnerFixture(t)
	var sequence []string
	runner.operations = lifecycleOperations{
		undeploy: func(kindrig.DeployRequest) error {
			sequence = append(sequence, "undeploy")
			return nil
		},
		deleteNamespace: func(kindrig.ApplicationNamespaceRequest) error {
			sequence = append(sequence, "delete-namespace")
			return nil
		},
	}
	runner.Probes = func(Resolved) StatusProbes {
		return StatusProbes{
			Bucket: func() ComponentStatus {
				return ComponentStatus{State: StateOK, Detail: "retained"}
			},
			QueryEndpoint: func() ComponentStatus {
				return ComponentStatus{State: StateAbsent, Detail: "collector removed"}
			},
		}
	}
	report, err := runner.Down()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sequence, []string{"undeploy", "delete-namespace"}) {
		t.Fatalf("down sequence = %v", sequence)
	}
	if report.Bucket.State != StateOK || report.QueryEndpoint.State != StateAbsent {
		t.Fatalf("down report conflated retained data and query compute: %+v", report)
	}
}

func TestRunnerDiagnoseAndPurgeDelegation(t *testing.T) {
	runner := runnerFixture(t)
	diagnosed := false
	runner.operations.diagnose = func(request kindrig.DiagnoseRequest) error {
		diagnosed = request.Scenario == "fixture" && request.Agent.Binary == "agent"
		return nil
	}
	if err := runner.Diagnose(); err != nil || !diagnosed {
		t.Fatalf("diagnose = %v, delegated=%t", err, diagnosed)
	}
	if err := runner.PurgeData("purge:fixture"); !errors.Is(err, ErrPurgeUnavailable) {
		t.Fatalf("unconfigured purge = %v", err)
	}
	purged := false
	runner.Purge = func(resolved Resolved, confirmation string) error {
		purged = resolved.BucketURL == "gs://fixture-telemetry" && confirmation == "purge:fixture"
		return nil
	}
	if err := runner.PurgeData("purge:fixture"); err != nil || !purged {
		t.Fatalf("configured purge = %v, delegated=%t", err, purged)
	}
}
