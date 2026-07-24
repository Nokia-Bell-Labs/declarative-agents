// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestChatbotRequiredModelsResolveShippedDefaults(t *testing.T) {
	for _, name := range []string{
		"CORPUS_EMBEDDING_MODEL",
		"CHATBOT_EMBEDDING_MODEL",
		"CHATBOT_ROUTER_MODEL",
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
	t.Setenv("CHATBOT_ROUTER_MODEL", "chatbot-router")
	t.Setenv("CHATBOT_FAST_MODEL", "chatbot-fast")
	t.Setenv("CHATBOT_DEEP_MODEL", "chatbot-deep")

	got, err := chatbotRequiredModels(filepath.Dir(findChartDir(t)))
	if err != nil {
		t.Fatalf("chatbotRequiredModels: %v", err)
	}
	want := []string{"chatbot-deep", "chatbot-embed", "chatbot-fast", "chatbot-router", "corpus-embed"}
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
