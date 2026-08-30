// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The chatbot's $tool tier selector never dispatched the word it selected, from
// rel01.0 until GH-75. Every turn answered on invoke_llm_fast through the
// ParseFailed fallback, including turns routed to invoke_llm_deep, and nothing
// showed it: the fallback returns a well-formed, corpus-grounded answer, and
// metadata.sources.composed is populated either way.
//
// Two independent conditions cause it, and these tests hold both. Each is
// checked against the profile rather than against a list of expected words,
// because a test carrying its own copy of the vocabulary passes when the
// vocabulary is wrong in both places -- which is the failure being fixed.

const (
	chatbotAgentDir           = "agents/chatbot"
	chatbotRequestMachine     = "request-machine.yaml"
	chatbotRequestToolsFile   = "request-tools.yaml"
	chatbotPersistentTools    = "tools.yaml"
	chatbotDispatchSignal     = "ToolDone"
	chatbotDispatchAction     = "$tool"
	chatbotExternalVisibility = "external"
)

type chatbotToolDeclaration struct {
	Name       string   `yaml:"name"`
	Visibility string   `yaml:"visibility"`
	Emits      []string `yaml:"emits"`
}

type chatbotDeclarationFile struct {
	Tools []chatbotToolDeclaration `yaml:"tools"`
}

type chatbotToolSelection struct {
	Tools []string `yaml:"tools"`
}

type chatbotTransition struct {
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action"`
	Label  string `yaml:"label"`
}

type chatbotMachine struct {
	Transitions []chatbotTransition `yaml:"transitions"`
}

func chatbotAgentPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(agentDir(t, "chatbot"), name)
}

func readChatbotYAML(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

// requestDeclarations returns the request machine's tool declarations, which is
// where the chat vocabulary lives.
func requestDeclarations(t *testing.T) []chatbotToolDeclaration {
	t.Helper()
	var file chatbotDeclarationFile
	readChatbotYAML(t, chatbotAgentPath(t, "request-declarations.yaml"), &file)
	if len(file.Tools) == 0 {
		t.Fatal("request-declarations.yaml declares no tools")
	}
	return file.Tools
}

// dispatchableWords are the externally-visible words. requestToolDefs in
// agent-core registers the words a machine's transitions name, and adds the
// externally-visible ones only when they also appear in the profile's
// selection, so this set is exactly what a $tool transition can reach.
func dispatchableWords(declarations []chatbotToolDeclaration) []string {
	var names []string
	for _, declaration := range declarations {
		if declaration.Visibility == chatbotExternalVisibility {
			names = append(names, declaration.Name)
		}
	}
	sort.Strings(names)
	return names
}

func toolSelection(t *testing.T, file string) map[string]bool {
	t.Helper()
	var selection chatbotToolSelection
	readChatbotYAML(t, chatbotAgentPath(t, file), &selection)
	selected := make(map[string]bool, len(selection.Tools))
	for _, name := range selection.Tools {
		selected[name] = true
	}
	return selected
}

// TestTierDispatchWordsAreSelectableByTheRequestMachine fails when a word a
// $tool transition can select is missing from the request profile's selection.
// An unselected word is never registered, so the selector's choice resolves as
// unavailable and the turn answers on the fallback.
func TestTierDispatchWordsAreSelectableByTheRequestMachine(t *testing.T) {
	words := dispatchableWords(requestDeclarations(t))
	if len(words) == 0 {
		t.Fatal("no externally-visible words declared; the $tool selector would have nothing to dispatch")
	}
	selected := toolSelection(t, chatbotRequestToolsFile)
	for _, word := range words {
		if !selected[word] {
			t.Errorf("%s declares %s externally visible, but %s does not select it,"+
				" so a tier selection of that word resolves as unavailable and the turn falls back",
				"request-declarations.yaml", word, chatbotRequestToolsFile)
		}
	}
}

// TestTierDispatchWordsStayOutOfThePersistentSelection fails when a chat word
// appears in the persistent agent's selection. A persistent agent that
// registers an invoke_llm word inherits that word's max_time as the whole run's
// budget and cancels itself mid-wait (GH-86, and GH-88 upstream). The comment in
// tools.yaml is otherwise the only thing preventing it.
func TestTierDispatchWordsStayOutOfThePersistentSelection(t *testing.T) {
	words := dispatchableWords(requestDeclarations(t))
	persistent := toolSelection(t, chatbotPersistentTools)
	for _, word := range words {
		if persistent[word] {
			t.Errorf("%s selects the chat word %s; the persistent agent then registers it and its"+
				" max_time becomes the agent's run budget, so the agent cancels itself while idle (GH-86)",
				chatbotPersistentTools, word)
		}
	}
}

// TestTierDispatchTransitionAgreesWithTheParseState fails when the $tool
// transition's target differs from the state the parse word's transition
// targets. A dispatchable word is scoped to the manifest of the state $tool
// targets, while the parse word validates the selection in the state its own
// transition targets. When the two disagree every selection resolves as
// unavailable, and the turn answers on the fallback with nothing to say so.
func TestTierDispatchTransitionAgreesWithTheParseState(t *testing.T) {
	var machine chatbotMachine
	readChatbotYAML(t, chatbotAgentPath(t, chatbotRequestMachine), &machine)

	parseWord := ""
	for _, declaration := range requestDeclarations(t) {
		for _, signal := range declaration.Emits {
			if signal == chatbotDispatchSignal {
				parseWord = declaration.Name
			}
		}
	}
	if parseWord == "" {
		t.Fatalf("no declared word emits %s, so nothing validates a tier selection", chatbotDispatchSignal)
	}

	dispatchTarget, parseTarget := "", ""
	for _, transition := range machine.Transitions {
		switch transition.Action {
		case chatbotDispatchAction:
			dispatchTarget = transition.Next
		case parseWord:
			parseTarget = transition.Next
		}
	}
	if dispatchTarget == "" {
		t.Fatalf("%s declares no %s transition", chatbotRequestMachine, chatbotDispatchAction)
	}
	if parseTarget == "" {
		t.Fatalf("%s declares no transition whose action is %s", chatbotRequestMachine, parseWord)
	}
	if dispatchTarget != parseTarget {
		t.Errorf("the %s transition targets %q while %s validates the selection in %q;"+
			" a dispatchable word is scoped to the first and checked in the second, so every"+
			" selection resolves as unavailable and the turn falls back",
			chatbotDispatchAction, dispatchTarget, parseWord, parseTarget)
	}
}

// The vocabulary is derived, not listed, so this pins that the derivation finds
// the words a reader would expect. A change here is a real change to what the
// selector can dispatch.
func TestTierDispatchVocabularyIsDerivedFromTheDeclarations(t *testing.T) {
	words := dispatchableWords(requestDeclarations(t))
	if got := strings.Join(words, ","); got != "invoke_llm_deep,invoke_llm_fast" {
		t.Fatalf("dispatchable words = %q; if this changed deliberately, update the expectation", got)
	}
}
