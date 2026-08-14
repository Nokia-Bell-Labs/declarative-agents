// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type declaredUndoContract struct {
	Tools []struct {
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
	} `yaml:"tools"`
}

type declaredRESTRollbackConfig struct {
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

func TestMutationUndoContractsStaySemanticallyAligned(t *testing.T) {
	type expectedContract struct {
		file, name, classification, strategy, payload string
		confirmation                                  bool
	}
	cases := []expectedContract{
		{"../../catalog/agents/applier/declarations.yaml", "await_applier_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/chatbot/declarations.yaml", "await_chatbot_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../../catalog/agents/collector/declarations.yaml", "await_collector_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/provisioning-workflow-orchestrator/declarations.yaml", "await_provisioning_workflow_orchestrator_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/creator/declarations.yaml", "await_creator_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/rag-server/declarations.yaml", "await_rag_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../../catalog/agents/applier/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../../catalog/agents/applier/declarations.yaml", "stop_applier_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/chatbot/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/chatbot/declarations.yaml", "stop_chat_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/provisioning-workflow-orchestrator/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/provisioning-workflow-orchestrator/declarations.yaml", "stop_provisioning_workflow_orchestrator_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/creator/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/creator/declarations.yaml", "stop_creator_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/rag-server/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/rag-server/declarations.yaml", "stop_rag_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../../catalog/agents/collector/declarations.yaml", "stop_collector_monitor", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../../catalog/agents/collector/declarations.yaml", "stop_collector_control", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../../catalog/agents/collector/declarations.yaml", "spool_collector_spans", "irreversible", "irreversible", "", true},
		{"../../catalog/agents/collector/declarations.yaml", "relay_collector_spans", "irreversible", "irreversible", "", true},
		{"../agents/applier/exec-declarations.yaml", "helm_rollback", "irreversible", "irreversible", "", true},
		{"../agents/rag-server/request-declarations.yaml", "rag_resolve", "irreversible", "irreversible", "", true},
		{"../agents/provisioning-workflow-orchestrator/request-declarations.yaml", "request_ingest", "irreversible", "irreversible", "", true},
		{"../agents/provisioning-workflow-orchestrator/request-declarations.yaml", "request_rollout", "irreversible", "irreversible", "", true},
		{"../agents/provisioning-workflow-orchestrator/request-declarations.yaml", "request_rollout_values", "irreversible", "irreversible", "", true},
		{"../agents/creator/request-declarations.yaml", "apply_instance", "irreversible", "irreversible", "", true},
		{"../agents/creator/request-declarations.yaml", "run_corpus_ingest", "irreversible", "irreversible", "", true},
		{"../../../agent-core/tools/builtin/otlp/all.yaml", "await_spans", "irreversible", "irreversible", "", true},
		{"../../../agent-core/tools/builtin/otlp/all.yaml", "spool_spans", "irreversible", "irreversible", "", true},
		{"../../../agent-core/tools/builtin/otlp/all.yaml", "relay_spans", "irreversible", "irreversible", "", true},
		{"../../../agent-core/tools/builtin/otlp/all.yaml", "otlp_receiver_stop", "irreversible", "irreversible", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := readDeclaredUndoTool(t, tc.file, tc.name)
			if tool.Reversibility.Classification != tc.classification {
				t.Errorf("classification = %q, want %q",
					tool.Reversibility.Classification, tc.classification)
			}
			if tool.Undo.Strategy != tc.strategy {
				t.Errorf("undo strategy = %q, want %q", tool.Undo.Strategy, tc.strategy)
			}
			if tool.Undo.Payload != tc.payload {
				t.Errorf("undo payload = %q, want %q", tool.Undo.Payload, tc.payload)
			}
			if tool.Reversibility.RequiresConfirmation != tc.confirmation {
				t.Errorf("requires_confirmation = %v, want %v",
					tool.Reversibility.RequiresConfirmation, tc.confirmation)
			}
			if tc.payload != "" && len(tool.Undo.Captures) == 0 {
				t.Error("receipt-consuming undo has no captures")
			}
		})
	}
}

func TestCreatorApplyRESTPolicyMatchesIrreversibleTool(t *testing.T) {
	var config declaredRESTRollbackConfig
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &config)
	operation := config.Rest.Clients["deployment_api"].Operations["apply_instance"]
	if operation.Reversibility.Classification != "irreversible" ||
		operation.Reversibility.Undo != "irreversible" ||
		!operation.Reversibility.RequiresConfirmation {
		t.Errorf("creator apply REST policy = %+v, want confirmed irreversible",
			operation.Reversibility)
	}
	if len(operation.Compensation) != 0 {
		t.Errorf("creator apply declares compensation without prior deployment state: %v",
			operation.Compensation)
	}
}

func TestRequestIngestRESTPolicyMatchesIrreversibleTool(t *testing.T) {
	var config declaredRESTRollbackConfig
	readIntakeYAML(t, filepath.Join(agentDir(t, "provisioning-workflow-orchestrator"), "rest.yaml"), &config)
	operation := config.Rest.Clients["creator"].Operations["creator_ingest"]
	if operation.Reversibility.Classification != "irreversible" ||
		operation.Reversibility.Undo != "irreversible" ||
		!operation.Reversibility.RequiresConfirmation {
		t.Errorf("creator ingest REST policy = %+v, want confirmed irreversible",
			operation.Reversibility)
	}
	if len(operation.Compensation) != 0 {
		t.Errorf("creator ingest declares unavailable compensation: %v", operation.Compensation)
	}
}

func TestCorpusIngestAddRESTPolicyProducesCompensation(t *testing.T) {
	var config declaredRESTRollbackConfig
	readIntakeYAML(t, filepath.Join(agentDir(t, "corpus-ingest"), "corpus-rest.yaml"), &config)
	operations := config.Rest.Clients["chroma"].Operations
	add := operations["add_records"]
	if add.Reversibility.Classification != "compensatable" ||
		add.Reversibility.Undo != "delete_records" ||
		add.Compensation["operation"] != "delete_records" {
		t.Errorf("Chroma add REST policy = %+v compensation=%v, want delete-records compensation",
			add.Reversibility, add.Compensation)
	}
	remove := operations["delete_records"]
	if remove.Reversibility.Classification != "irreversible" ||
		remove.Reversibility.Undo != "irreversible" ||
		!remove.Reversibility.RequiresConfirmation {
		t.Errorf("Chroma delete REST policy = %+v, want confirmed irreversible",
			remove.Reversibility)
	}
}

func readDeclaredUndoTool(
	t *testing.T,
	path, name string,
) *struct {
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
} {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var declarations declaredUndoContract
	if err := yaml.Unmarshal(data, &declarations); err != nil {
		t.Fatal(err)
	}
	for index := range declarations.Tools {
		if declarations.Tools[index].Name == name {
			return &declarations.Tools[index]
		}
	}
	t.Fatalf("tool %q not found in %s", name, path)
	return nil
}
