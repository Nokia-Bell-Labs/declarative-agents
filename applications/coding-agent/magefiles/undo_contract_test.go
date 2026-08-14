// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// These assert the applier's mutation-undo contracts (srd006, agent-core srd028).
// The load-bearing one is the compensating pair: helm_upgrade must declare a
// compensating strategy because apply-machine.yaml routes a post-apply verify
// stall through helm_rollback.

type declaredUndoTool struct {
	Name          string `yaml:"name"`
	Reversibility struct {
		Classification       string `yaml:"classification"`
		RequiresConfirmation bool   `yaml:"requires_confirmation"`
	} `yaml:"reversibility"`
	Undo struct {
		Strategy string   `yaml:"strategy"`
		Payload  string   `yaml:"payload"`
		Captures []string `yaml:"captures"`
	} `yaml:"undo"`
}

type declaredUndoContract struct {
	Tools []declaredUndoTool `yaml:"tools"`
}

// TestApplierHelmRollbackIsTheUndoOfHelmUpgrade pins the compensating pair: the
// upgrade is compensatable, and the rollback is itself a one-way action that
// needs confirmation.
func TestApplierHelmRollbackIsTheUndoOfHelmUpgrade(t *testing.T) {
	exec := filepath.Join(agentDir(t, "applier"), "exec-declarations.yaml")

	upgrade := readDeclaredUndoTool(t, exec, "helm_upgrade")
	if upgrade.Reversibility.Classification != "compensatable" {
		t.Errorf("helm_upgrade classification = %q, want compensatable", upgrade.Reversibility.Classification)
	}
	if upgrade.Undo.Strategy != "compensating_action" {
		t.Errorf("helm_upgrade undo strategy = %q, want compensating_action", upgrade.Undo.Strategy)
	}

	rollback := readDeclaredUndoTool(t, exec, "helm_rollback")
	if rollback.Reversibility.Classification != "irreversible" {
		t.Errorf("helm_rollback classification = %q, want irreversible", rollback.Reversibility.Classification)
	}
	if rollback.Undo.Strategy != "irreversible" {
		t.Errorf("helm_rollback undo strategy = %q, want irreversible", rollback.Undo.Strategy)
	}
	if !rollback.Reversibility.RequiresConfirmation {
		t.Error("helm_rollback does not require confirmation; a one-way compensating action must")
	}
}

// TestApplierMutationUndoContractsStaySemanticallyAligned covers the applier's
// remaining mutating words, so a lifecycle or boundary word cannot quietly lose
// its declared compensation.
func TestApplierMutationUndoContractsStaySemanticallyAligned(t *testing.T) {
	type expectedContract struct {
		file, name, classification, strategy, payload string
		confirmation                                  bool
	}
	decls := filepath.Join("..", "..", "catalog", "agents", "applier", "declarations.yaml")
	exec := filepath.Join(agentDir(t, "applier"), "exec-declarations.yaml")
	cases := []expectedContract{
		{decls, "await_applier_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{decls, "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{decls, "stop_applier_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{exec, "helm_rollback", "irreversible", "irreversible", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := readDeclaredUndoTool(t, tc.file, tc.name)
			if tool.Reversibility.Classification != tc.classification {
				t.Errorf("classification = %q, want %q", tool.Reversibility.Classification, tc.classification)
			}
			if tool.Undo.Strategy != tc.strategy {
				t.Errorf("undo strategy = %q, want %q", tool.Undo.Strategy, tc.strategy)
			}
			if tool.Undo.Payload != tc.payload {
				t.Errorf("undo payload = %q, want %q", tool.Undo.Payload, tc.payload)
			}
			if tool.Reversibility.RequiresConfirmation != tc.confirmation {
				t.Errorf("requires_confirmation = %v, want %v", tool.Reversibility.RequiresConfirmation, tc.confirmation)
			}
			if tc.payload != "" && len(tool.Undo.Captures) == 0 {
				t.Error("receipt-consuming undo has no captures")
			}
		})
	}
}

func TestPlannerExecuteRESTPolicyMatchesIrreversibleTool(t *testing.T) {
	planner := agentDir(t, "planner")
	tool := readDeclaredUndoTool(t, filepath.Join(planner, "request-declarations.yaml"), "delegate_executor")
	if tool.Reversibility.Classification != "irreversible" ||
		!tool.Reversibility.RequiresConfirmation ||
		tool.Undo.Strategy != "irreversible" {
		t.Errorf("delegate executor policy = %+v, want confirmed irreversible", tool)
	}

	var config struct {
		Rest struct {
			Clients map[string]struct {
				Operations map[string]struct {
					Compensation  map[string]interface{} `yaml:"compensation"`
					Reversibility struct {
						Classification       string `yaml:"classification"`
						Undo                 string `yaml:"undo"`
						RequiresConfirmation bool   `yaml:"requires_confirmation"`
					} `yaml:"reversibility"`
				} `yaml:"operations"`
			} `yaml:"clients"`
		} `yaml:"rest"`
	}
	readIntakeYAML(t, filepath.Join(planner, "rest.yaml"), &config)
	operation := config.Rest.Clients["executor"].Operations["execute"]
	if operation.Reversibility.Classification != "irreversible" ||
		operation.Reversibility.Undo != "irreversible" ||
		!operation.Reversibility.RequiresConfirmation {
		t.Errorf("planner execute REST policy = %+v, want confirmed irreversible",
			operation.Reversibility)
	}
	if len(operation.Compensation) != 0 {
		t.Errorf("planner execute declares compensation without a restore endpoint: %v",
			operation.Compensation)
	}
}

func readDeclaredUndoTool(t *testing.T, path, name string) declaredUndoTool {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var declarations declaredUndoContract
	if err := yaml.Unmarshal(data, &declarations); err != nil {
		t.Fatal(err)
	}
	for _, tool := range declarations.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found in %s", name, path)
	return declaredUndoTool{}
}
