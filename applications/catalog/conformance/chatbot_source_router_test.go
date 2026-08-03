// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

const chatbotSourceRouterModel = "qwen2.5:3b"

// TestChatbotSourceRouterConformance checks the profile-owned classifier prompt
// against the same live model used by the chatbot's source selector. It is
// intentionally separate from tier selection: this case asks for one scoped
// corpus and validates the names-only structured contract.
func TestChatbotSourceRouterConformance(t *testing.T) {
	t.Parallel()
	liveTimeout := RequireLiveModel(t, chatbotSourceRouterModel)
	root := filepath.Join("..", "..", "chatbot-mesh", "agents", "chatbot")
	data, err := os.ReadFile(filepath.Join(root, "request-declarations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var declarations struct {
		Tools []struct {
			Name   string `yaml:"name"`
			Config struct {
				SystemPrompt string `yaml:"system_prompt"`
			} `yaml:"config"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &declarations); err != nil {
		t.Fatal(err)
	}
	var system string
	for _, tool := range declarations.Tools {
		if tool.Name == "select_sources" {
			system = tool.Config.SystemPrompt
			break
		}
	}
	if system == "" {
		t.Fatal("select_sources system prompt not found")
	}
	request := map[string]any{
		"model":  chatbotSourceRouterModel,
		"stream": false,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": "Question:\nHow does the scenario critic rig validate plans?\n\nDeclared source catalog:\n- rag0: Primary declarative-agent and scenario critic rig behavior.\n- rag1: Secondary mock-dependency and Solar Ridge integration fixtures."},
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: liveTimeout}
	resp, err := client.Post(ollamaBaseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Ollama chat returned %d", resp.StatusCode)
	}
	var chat struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chat); err != nil {
		t.Fatal(err)
	}
	var selection struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal([]byte(chat.Message.Content), &selection); err != nil {
		t.Fatalf("source router returned malformed structured output %q: %v", chat.Message.Content, err)
	}
	var exact map[string]json.RawMessage
	if err := json.Unmarshal([]byte(chat.Message.Content), &exact); err != nil ||
		len(exact) != 1 || exact["names"] == nil {
		t.Fatalf("source router returned fields outside the names-only contract: %q", chat.Message.Content)
	}
	if len(selection.Names) != 1 || selection.Names[0] != "rag0" {
		t.Fatalf("scoped source selection = %v, want [rag0]", selection.Names)
	}
}
