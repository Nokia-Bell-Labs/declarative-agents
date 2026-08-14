// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCitedRecordNumbersBracketed(t *testing.T) {
	answer := "The project has a capacity of 88 megawatts [record 1], produced by 22 turbines [record 3]."
	got := citedRecordNumbers(answer)
	want := []int{1, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("citedRecordNumbers = %v, want %v", got, want)
	}
}

func TestCitedRecordNumbersDedupesAndSorts(t *testing.T) {
	answer := "See record 2 and Record #2, and also RECORD 1."
	got := citedRecordNumbers(answer)
	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("citedRecordNumbers = %v, want %v", got, want)
	}
}

func TestCitedRecordNumbersUngrounded(t *testing.T) {
	answer := "The retrieved chunks do not contain the answer, so I cannot reference them."
	if got := citedRecordNumbers(answer); len(got) != 0 {
		t.Fatalf("citedRecordNumbers on ungrounded answer = %v, want empty", got)
	}
}

func TestChatResponseDecodesTrace(t *testing.T) {
	var resp chatResponse
	if err := json.Unmarshal([]byte(`{"answer":"grounded [record 1]","trace":{"status":"succeeded","terminal_signal":"LLMResponded"}}`), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Answer == "" {
		t.Fatalf("answer is empty")
	}
	if resp.Trace.Status != "succeeded" {
		t.Fatalf("trace.status = %q, want succeeded", resp.Trace.Status)
	}
	if got := citedRecordNumbers(resp.Answer); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("citedRecordNumbers = %v, want [1]", got)
	}
}

func TestAssertChatbotTierSelectionTraceAttributesAnswerDispatch(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		chatbotSpanLine("machine_request chatbot/chat", "turn-fast", "", nil),
		chatbotSpanLine("chat qwen-fast", "source-selector", "turn-fast", map[string]string{"gen_ai.request.model": "qwen-fast"}),
		chatbotSpanLine("chat qwen-fast", "tier-selector", "turn-fast", map[string]string{"gen_ai.request.model": "qwen-fast"}),
		chatbotSpanLine("execute_tool parse_tier", "fast-parse", "turn-fast", map[string]string{"command.name": "parse_tier"}),
		chatbotSpanLine("chat qwen-fast", "late-tier-selector", "turn-fast", map[string]string{"command.name": "select_tier", "gen_ai.request.model": "qwen-fast"}),
		chatbotSpanLine("chat qwen-fast", "fast-answer", "turn-fast", map[string]string{"command.name": "invoke_llm_fast", "gen_ai.request.model": "qwen-fast"}),
		chatbotSpanLine("execute_tool compose_response", "fast-compose", "turn-fast", map[string]string{"command.name": "compose_response"}),
		chatbotSpanLine("machine_request chatbot/chat", "turn-deep", "", nil),
		chatbotSpanLine("chat qwen-fast", "deep-selector", "turn-deep", map[string]string{"gen_ai.request.model": "qwen-fast"}),
		chatbotSpanLine("execute_tool parse_tier", "deep-parse", "turn-deep", map[string]string{"command.name": "parse_tier"}),
		chatbotSpanLine("chat ornith-deep", "deep-answer", "turn-deep", map[string]string{"command.name": "invoke_llm_deep", "gen_ai.request.model": "ornith-deep"}),
		chatbotSpanLine("execute_tool compose_response", "deep-compose", "turn-deep", map[string]string{"command.name": "compose_response"}),
	})
	if err := assertChatbotTierSelectionTrace(trace, "qwen-fast", "ornith-deep"); err != nil {
		t.Fatalf("expected attributed fast and deep answer dispatches to pass: %v", err)
	}
}

func TestAssertChatbotTierSelectionTraceDoesNotRequireBothTiers(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		chatbotSpanLine("machine_request chatbot/chat", "turn-1", "", nil),
		chatbotSpanLine("execute_tool parse_tier", "parse-1", "turn-1", map[string]string{"command.name": "parse_tier"}),
		chatbotSpanLine("chat qwen-fast", "answer-1", "turn-1", map[string]string{"command.name": "invoke_llm_fast", "gen_ai.request.model": "qwen-fast"}),
		chatbotSpanLine("execute_tool compose_response", "compose-1", "turn-1", map[string]string{"command.name": "compose_response"}),
		chatbotSpanLine("machine_request chatbot/chat", "turn-2", "", nil),
		chatbotSpanLine("execute_tool parse_tier", "parse-2", "turn-2", map[string]string{"command.name": "parse_tier"}),
		chatbotSpanLine("chat qwen-fast", "answer-2", "turn-2", map[string]string{"command.name": "invoke_llm_fast", "gen_ai.request.model": "qwen-fast"}),
		chatbotSpanLine("execute_tool compose_response", "compose-2", "turn-2", map[string]string{"command.name": "compose_response"}),
	})
	if err := assertChatbotTierSelectionTrace(trace, "qwen-fast", "ornith-deep"); err != nil {
		t.Fatalf("two model-dependent fast selections should pass: %v", err)
	}
}

func TestAssertChatbotTierSelectionTraceRejectsInvalidAttribution(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		wantErr string
	}{
		{
			name: "selector-only Qwen is not an answer dispatch",
			lines: []string{
				chatbotSpanLine("machine_request chatbot/chat", "turn", "", nil),
				chatbotSpanLine("chat qwen-fast", "selector", "turn", map[string]string{"gen_ai.request.model": "qwen-fast"}),
				chatbotSpanLine("execute_tool parse_tier", "parse", "turn", map[string]string{"command.name": "parse_tier"}),
				chatbotSpanLine("execute_tool compose_response", "compose", "turn", map[string]string{"command.name": "compose_response"}),
			},
			wantErr: "0 answer-word dispatch spans",
		},
		{
			name: "undeclared answer model",
			lines: []string{
				chatbotSpanLine("machine_request chatbot/chat", "turn", "", nil),
				chatbotSpanLine("execute_tool parse_tier", "parse", "turn", map[string]string{"command.name": "parse_tier"}),
				chatbotSpanLine("chat other", "answer", "turn", map[string]string{"command.name": "invoke_llm_fast", "gen_ai.request.model": "other"}),
				chatbotSpanLine("execute_tool compose_response", "compose", "turn", map[string]string{"command.name": "compose_response"}),
			},
			wantErr: "undeclared model",
		},
		{
			name: "two answer words",
			lines: []string{
				chatbotSpanLine("machine_request chatbot/chat", "turn", "", nil),
				chatbotSpanLine("execute_tool parse_tier", "parse", "turn", map[string]string{"command.name": "parse_tier"}),
				chatbotSpanLine("chat qwen-fast", "fast", "turn", map[string]string{"command.name": "invoke_llm_fast", "gen_ai.request.model": "qwen-fast"}),
				chatbotSpanLine("chat ornith-deep", "deep", "turn", map[string]string{"command.name": "invoke_llm_deep", "gen_ai.request.model": "ornith-deep"}),
				chatbotSpanLine("execute_tool compose_response", "compose", "turn", map[string]string{"command.name": "compose_response"}),
			},
			wantErr: "2 answer-word dispatch spans",
		},
		{
			name: "answer dispatch without model",
			lines: []string{
				chatbotSpanLine("machine_request chatbot/chat", "turn", "", nil),
				chatbotSpanLine("execute_tool parse_tier", "parse", "turn", map[string]string{"command.name": "parse_tier"}),
				chatbotSpanLine("chat unknown", "answer", "turn", map[string]string{"command.name": "invoke_llm_fast"}),
				chatbotSpanLine("execute_tool compose_response", "compose", "turn", map[string]string{"command.name": "compose_response"}),
			},
			wantErr: "has no gen_ai.request.model",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := assertChatbotTierSelectionTrace(writeChromaTrace(t, tt.lines), "qwen-fast", "ornith-deep")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func chatbotSpanLine(name, spanID, parentID string, attrs map[string]string) string {
	attributes := make([]map[string]interface{}, 0, len(attrs))
	for key, value := range attrs {
		attributes = append(attributes, map[string]interface{}{
			"Key": key, "Value": map[string]interface{}{"Type": "STRING", "Value": value},
		})
	}
	line, err := json.Marshal(map[string]interface{}{
		"Name":        name,
		"SpanContext": map[string]string{"TraceID": "trace", "SpanID": spanID},
		"Parent":      map[string]string{"TraceID": "trace", "SpanID": parentID},
		"Attributes":  attributes,
	})
	if err != nil {
		panic(err)
	}
	return string(line)
}

func TestChatbotRequiredModelsResolveShippedDefaults(t *testing.T) {
	for _, name := range []string{
		"CORPUS_EMBEDDING_MODEL",
		"CHATBOT_EMBEDDING_MODEL",
		"CHATBOT_TIER_MODEL",
		"CHATBOT_FAST_MODEL",
		"CHATBOT_DEEP_MODEL",
	} {
		unsetTestEnv(t, name)
	}

	got, err := chatbotRequiredModels(filepath.Dir(findChartDir(t)))
	if err != nil {
		t.Fatalf("chatbotRequiredModels: %v", err)
	}
	want := []string{"ornith:9b", "qwen2.5:3b", "qwen3-embedding:8b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("required models = %v, want shipped defaults %v", got, want)
	}
}

func TestChatbotRequiredModelsUseDeploymentEnvironment(t *testing.T) {
	t.Setenv("CORPUS_EMBEDDING_MODEL", "corpus-embed")
	t.Setenv("CHATBOT_EMBEDDING_MODEL", "chatbot-embed")
	t.Setenv("CHATBOT_TIER_MODEL", "chatbot-tier")
	t.Setenv("CHATBOT_FAST_MODEL", "chatbot-fast")
	t.Setenv("CHATBOT_DEEP_MODEL", "chatbot-deep")

	got, err := chatbotRequiredModels(filepath.Dir(findChartDir(t)))
	if err != nil {
		t.Fatalf("chatbotRequiredModels: %v", err)
	}
	want := []string{
		"chatbot-deep", "chatbot-embed", "chatbot-fast", "chatbot-tier",
		"corpus-embed", "qwen2.5:3b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("required models = %v, want environment-selected %v", got, want)
	}
}

func TestResolveModelReferenceLeavesLiteralName(t *testing.T) {
	const model = "qwen2.5:3b"
	if got := resolveModelReference(model); got != model {
		t.Fatalf("resolveModelReference(%q) = %q", model, got)
	}
}

func unsetTestEnv(t *testing.T, name string) {
	t.Helper()
	value, set := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if set {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}
