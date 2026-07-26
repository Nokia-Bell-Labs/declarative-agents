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

func TestMutationUndoContractsStaySemanticallyAligned(t *testing.T) {
	type expectedContract struct {
		file, name, classification, strategy, payload string
		confirmation                                  bool
	}
	cases := []expectedContract{
		{"../agents/applier/declarations.yaml", "await_applier_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/chatbot/declarations.yaml", "await_chatbot_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/collector/declarations.yaml", "await_collector_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/coordinator/declarations.yaml", "await_coordinator_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/creator/declarations.yaml", "await_creator_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/rag-server/declarations.yaml", "await_rag_control", "reversible", "queue_event_restore", "rest_await_event", false},
		{"../agents/applier/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/applier/declarations.yaml", "stop_applier_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/chatbot/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/chatbot/declarations.yaml", "stop_chat_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/coordinator/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/coordinator/declarations.yaml", "stop_coordinator_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/creator/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/creator/declarations.yaml", "stop_creator_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/rag-server/declarations.yaml", "stop_monitor_rest", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/rag-server/declarations.yaml", "stop_rag_requests", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/collector/declarations.yaml", "stop_collector_monitor", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/collector/declarations.yaml", "stop_collector_control", "compensatable", "server_shutdown_or_user_action_compensation", "boundary_compensation", false},
		{"../agents/collector/declarations.yaml", "spool_collector_spans", "irreversible", "irreversible", "", true},
		{"../agents/collector/declarations.yaml", "relay_collector_spans", "irreversible", "irreversible", "", true},
		{"../agents/applier/exec-declarations.yaml", "helm_rollback", "irreversible", "irreversible", "", true},
		{"../agents/rag-server/request-declarations.yaml", "rag_resolve", "irreversible", "irreversible", "", true},
		{"../agents/coordinator/request-declarations.yaml", "request_rollout", "irreversible", "irreversible", "", true},
		{"../agents/coordinator/request-declarations.yaml", "request_rollout_values", "irreversible", "irreversible", "", true},
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
