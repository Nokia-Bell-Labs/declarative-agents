// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
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
