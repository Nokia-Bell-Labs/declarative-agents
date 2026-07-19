// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// chatbotMachine is the subset of the chatbot request-machine needed to assert the
// fan-out relocation (GH-365): degradation and embedding-mismatch exclusion are
// visible machine transitions, not a merge word.
type chatbotMachine struct {
	States      []struct{ Name string } `yaml:"states"`
	Signals     []struct{ Name string } `yaml:"signals"`
	Transitions []struct {
		State  string `yaml:"state"`
		Signal string `yaml:"signal"`
		Next   string `yaml:"next"`
		Action string `yaml:"action"`
	} `yaml:"transitions"`
}

func loadChatbotMachine(t *testing.T) chatbotMachine {
	t.Helper()
	path := filepath.Join("..", "agents", "chatbot", "request-machine.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("chatbot request-machine.yaml not found: %v", err)
	}
	var m chatbotMachine
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse request-machine.yaml: %v", err)
	}
	return m
}

// TestChatbotFanOutHasNoMergeWord locks that rag_merge is gone from the chatbot
// turn: no Merging state, no Merged signal, no rag_merge action.
func TestChatbotFanOutHasNoMergeWord(t *testing.T) {
	m := loadChatbotMachine(t)
	for _, s := range m.States {
		if s.Name == "Merging" {
			t.Error("Merging state still present; rag_merge should be gone (GH-365)")
		}
	}
	for _, s := range m.Signals {
		if s.Name == "Merged" {
			t.Error("Merged signal still present; rag_merge should be gone (GH-365)")
		}
	}
	for _, tr := range m.Transitions {
		if tr.Action == "rag_merge" {
			t.Errorf("rag_merge action still present at (%s,%s)", tr.State, tr.Signal)
		}
	}
}

// TestChatbotFanOutRoutesDegradedAndExcluded locks that each RAG state routes on
// via both CommandError (degraded, R3.2) and QueryRejected (embedding-mismatch
// excluded, R3.3), so both outcomes are visible machine transitions.
func TestChatbotFanOutRoutesDegradedAndExcluded(t *testing.T) {
	m := loadChatbotMachine(t)

	has := func(state, signal string) bool {
		for _, tr := range m.Transitions {
			if tr.State == state && tr.Signal == signal {
				return true
			}
		}
		return false
	}
	for _, state := range []string{"Retrieving0", "Retrieving1"} {
		for _, signal := range []string{"QueryResponded", "CommandError", "QueryRejected"} {
			if !has(state, signal) {
				t.Errorf("machine has no transition for (%s, %s); each RAG state must route degraded and excluded outcomes", state, signal)
			}
		}
	}
	// QueryRejected must be a declared signal.
	declared := false
	for _, s := range m.Signals {
		if s.Name == "QueryRejected" {
			declared = true
		}
	}
	if !declared {
		t.Error("QueryRejected signal is not declared")
	}
}

// TestChatbotComposeReadsEachRagSource locks that compose_prompt reads each RAG's
// documents directly (no rag_merge indirection) so a degraded/excluded source
// renders empty rather than failing the compose.
func TestChatbotComposeReadsEachRagSource(t *testing.T) {
	path := filepath.Join("..", "agents", "chatbot", "request-declarations.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("request-declarations.yaml not found: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "rag_merge") {
		t.Error("request-declarations.yaml still references rag_merge (GH-365)")
	}
	for _, sel := range []string{"$from(rag_query0).mapped.documents", "$from(rag_query1).mapped.documents"} {
		if !strings.Contains(text, sel) {
			t.Errorf("compose_prompt does not read %s directly", sel)
		}
	}
	if strings.Contains(text, "$from(rag_merge)") {
		t.Error("compose_prompt still reads $from(rag_merge)")
	}
}
