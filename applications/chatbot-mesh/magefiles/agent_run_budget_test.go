// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A persistent agent that registers an invoke_llm word inherits that word's
// max_time as its entire run budget, and cancels itself when the budget
// elapses. agent-core resolves it at registration --
// cmd/agent/factories_llm.go onModelResolved sets st.maxDuration = cfg.MaxTime,
// and main.go runBudget then overrides the machine's own budget with it -- so
// nothing in a profile can opt out. A machine declaring command_timeout: 11m
// still dies at the word's two minutes.
//
// GH-86 is what that costs. The chat words moved into the chatbot's tools
// selection so a $tool transition could dispatch them (GH-84), the persistent
// agent began registering them, and every deployed chatbot cancelled itself
// after two idle minutes and crash-looped. It reached a deployment because the
// symptom needs two minutes of idle to appear and every gate drives a turn
// immediately, then stops the agent seconds later.
//
// These read the shipped profiles rather than exercising a running agent: the
// property is decided at registration, and a test that had to wait out a
// two-minute budget to observe it would be one nobody runs.

// llmBoundedTools reports an agent's declared words whose init is invoke_llm
// and which carry a max_time, keyed by name. Those are the words that bound a
// run when a machine registers them.
func llmBoundedTools(t *testing.T, agentDir string) map[string]int {
	t.Helper()
	declarations, err := filepath.Glob(filepath.Join(agentDir, "*declarations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	bounded := map[string]int{}
	for _, path := range declarations {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var file struct {
			Tools []struct {
				Name   string `yaml:"name"`
				Init   string `yaml:"init"`
				Config struct {
					MaxTime int `yaml:"max_time"`
				} `yaml:"config"`
			} `yaml:"tools"`
		}
		if err := yaml.Unmarshal(data, &file); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, tool := range file.Tools {
			if tool.Init == "invoke_llm" && tool.Config.MaxTime > 0 {
				bounded[tool.Name] = tool.Config.MaxTime
			}
		}
	}
	return bounded
}

// profileToolSelection returns the word names a profile's tools files select.
func profileToolSelection(t *testing.T, agentDir, profileName string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(agentDir, profileName))
	if err != nil {
		t.Fatal(err)
	}
	var profile struct {
		Tools []string `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		t.Fatalf("%s: %v", profileName, err)
	}
	var selected []string
	for _, toolsFile := range profile.Tools {
		if strings.HasPrefix(toolsFile, "/") {
			continue // an agent-core path, not a profile-local selection
		}
		data, err := os.ReadFile(filepath.Join(agentDir, toolsFile))
		if err != nil {
			t.Fatal(err)
		}
		var file struct {
			Tools []string `yaml:"tools"`
		}
		if err := yaml.Unmarshal(data, &file); err != nil {
			t.Fatalf("%s: %v", toolsFile, err)
		}
		selected = append(selected, file.Tools...)
	}
	return selected
}

// TestPersistentAgentSelectsNoRunBoundingWord is the GH-86 regression. Every
// persistent agent profile in the mesh is checked, not just the chatbot: the
// trap is a property of the runtime, and the next agent to gain a chat word
// would hit it the same way.
func TestPersistentAgentSelectsNoRunBoundingWord(t *testing.T) {
	agents, err := filepath.Glob(filepath.Join("..", "agents", "*"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, agentDir := range agents {
		profilePath := filepath.Join(agentDir, "profile.yaml")
		if _, err := os.Stat(profilePath); err != nil {
			continue
		}
		declarations, err := filepath.Glob(filepath.Join(agentDir, "*declarations.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if len(declarations) == 0 {
			continue
		}
		agent := filepath.Base(agentDir)
		t.Run(agent, func(t *testing.T) {
			bounded := llmBoundedTools(t, agentDir)
			if len(bounded) == 0 {
				t.Skipf("%s declares no invoke_llm word carrying a max_time", agent)
			}
			for _, selected := range profileToolSelection(t, agentDir, "profile.yaml") {
				if maxTime, ok := bounded[selected]; ok {
					t.Errorf("the persistent profile selects %q, whose max_time of %ds becomes the"+
						" agent's entire run budget; it will cancel itself after that long idle."+
						" Move the word to a request-scoped tools selection (GH-86)",
						selected, maxTime)
				}
			}
		})
		checked++
	}
	if checked == 0 {
		t.Fatal("no agent profiles were checked; the glob found nothing")
	}
}

// The separation only works if the request machine still selects the words its
// $tool transition dispatches. Without them the tier selector's choice fails to
// resolve, the turn takes its fallback, and every turn looks healthy while the
// selected tier is unreachable -- the silent-fallback defect arrived at from
// the other side (GH-1900).
func TestChatbotRequestProfileSelectsTheDispatchVocabulary(t *testing.T) {
	agentDir := filepath.Join("..", "agents", "chatbot")
	selected := profileToolSelection(t, agentDir, "request-profile.yaml")
	have := map[string]bool{}
	for _, name := range selected {
		have[name] = true
	}
	for _, want := range []string{"invoke_llm_fast", "invoke_llm_deep"} {
		if !have[want] {
			t.Errorf("the request profile does not select %q, so a $tool transition cannot dispatch it"+
				" and the turn falls back silently (GH-84)", want)
		}
	}
}

// The chat endpoint has to load the request profile. Pointing it back at the
// agent's own profile restores the GH-86 failure without changing any tools
// file, which is the edit most likely to look harmless in review.
func TestChatEndpointLoadsTheRequestProfile(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "agents", "chatbot", "rest.yaml"),
		filepath.Join("..", "helm", "templates", "_chatbot-rest.tpl"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "profile: request-profile.yaml") {
			t.Errorf("%s does not bind machine_request to request-profile.yaml;"+
				" the persistent agent's tools selection would be used for the turn (GH-86)", path)
		}
	}
}
