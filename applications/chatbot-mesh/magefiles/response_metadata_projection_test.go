// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reportedSourceRenderers are the words that render the metadata.sources lists,
// and the only place a fan-out join entry may be turned into something the
// browser receives.
var reportedSourceRenderers = []string{
	"render_composed_sources",
	"render_model_excluded_sources",
	"render_query_failed_sources",
}

// TestReportedSourcesAreProjectedNotWholeJoinEntries proves the response reports
// projected source entries rather than the fan-out join entries themselves.
//
// A join entry pairs the query result with `input`, the topology entry the
// fan-out dispatched to, which carries base_url -- the in-cluster address of the
// RAG unit. compose_response composed those entries directly, so every answer
// told the browser where each RAG server lives, while
// compose_source_selection_prompt two states earlier withholds the same fields
// from a classifier that only had to choose source names (GH-216).
//
// The check is structural rather than a string search for base_url: the leak was
// not a field anyone wrote down, it was the shape of the value being composed,
// and the next one would be too.
func TestReportedSourcesAreProjectedNotWholeJoinEntries(t *testing.T) {
	body := readAgentFile(t, "chatbot", "request-fanout-declarations.yaml")

	// compose_response must read the rendered projections. Reading a partition
	// directly is the defect: a partition's items are join entries.
	for _, forbidden := range []string{
		"composed: $from(partition_embedding_models).matched",
		"model_excluded: $from(partition_embedding_models).unmatched",
		"query_failed: $from(partition_query_results).unmatched",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf(
				"compose_response reads %q, so the browser receives whole join entries"+
					" including the topology entry and its base_url", forbidden)
		}
	}
	for _, renderer := range reportedSourceRenderers {
		if !strings.Contains(body, "$from("+renderer+").$") {
			t.Errorf("compose_response does not read %s; a source list is still composed unprojected", renderer)
		}
	}

	// No renderer may emit the whole item or the whole input. Naming individual
	// input fields is how the source's own name is reported.
	for _, renderer := range reportedSourceRenderers {
		config := wordItemTemplate(t, body, renderer)
		if config == "" {
			t.Errorf("%s declares no item_template", renderer)
			continue
		}
		for _, forbidden := range []string{"json input }}", "json . }}", "base_url"} {
			if strings.Contains(config, forbidden) {
				t.Errorf(
					"%s renders %q, which carries the topology entry the fan-out"+
						" iterated into the browser's response", renderer, forbidden)
			}
		}
		if !strings.Contains(config, "input.name") {
			t.Errorf("%s does not report the source name, so an outcome cannot be attributed", renderer)
		}
	}
}

// TestResponseCompositionReadsLabelledAnswer proves the model result survives
// the three source renderers that execute before compose_response.
func TestResponseCompositionReadsLabelledAnswer(t *testing.T) {
	declarations := readAgentFile(t, "chatbot", "request-fanout-declarations.yaml")
	if strings.Contains(declarations, "answer: $.") {
		t.Error("compose_response reads the adjacent source renderer instead of the model answer")
	}
	if !strings.Contains(declarations, "answer: $from(answer).$") {
		t.Error("compose_response does not read the labelled model answer")
	}

	var machine chatbotMachine
	readChatbotYAML(t, chatbotAgentPath(t, chatbotRequestMachine), &machine)
	producers := 0
	for _, transition := range machine.Transitions {
		isDynamicAnswer := transition.Action == chatbotDispatchAction
		isFallbackAnswer := transition.Action == "invoke_llm_fast" &&
			(transition.State == "ParsingTier" || transition.State == "Answering")
		if !isDynamicAnswer && !isFallbackAnswer {
			continue
		}
		producers++
		if transition.Label != "answer" {
			t.Errorf("answer-producing transition (%s,%s,%s) has label %q, want answer",
				transition.State, transition.Signal, transition.Action, transition.Label)
		}
	}
	if producers != 5 {
		t.Errorf("answer-producing transitions = %d, want dynamic dispatch plus four fallbacks", producers)
	}
}

// wordItemTemplate returns the item_template line of one render_each word.
func wordItemTemplate(t *testing.T, body, word string) string {
	t.Helper()
	start := strings.Index(body, "- name: "+word+"\n")
	if start < 0 {
		t.Fatalf("%s is not declared", word)
	}
	rest := body[start:]
	if next := strings.Index(rest[1:], "\n  - name: "); next > 0 {
		rest = rest[:next]
	}
	for _, line := range strings.Split(rest, "\n") {
		if strings.Contains(line, "item_template:") {
			return line
		}
	}
	return ""
}

// TestServedUIReadsTheProjectedSourceShape keeps the panel and the mesh in step.
// The UI derives attribution from these entries, and mage demo:verify applies
// the same rule over the same fields, so a shape change that reached one and not
// the other would leave them disagreeing about whether a turn was attributed.
func TestServedUIReadsTheProjectedSourceShape(t *testing.T) {
	path := filepath.Join(agentDir(t, "chatbot"), "ui", "app", "src", "api.ts")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	api := string(raw)
	for _, stale := range []string{"input?.name", "structured_output"} {
		if strings.Contains(api, stale) {
			t.Errorf("the served UI still reads %q, which the mesh no longer sends", stale)
		}
	}
	for _, want := range []string{"outcome.name", "outcome.documents"} {
		if !strings.Contains(api, want) {
			t.Errorf("the served UI does not read %q from the projected entry", want)
		}
	}
}
