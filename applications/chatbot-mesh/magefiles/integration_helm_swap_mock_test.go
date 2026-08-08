// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHelmSwapMockClassifiesSourceTierAndAnswerRequests(t *testing.T) {
	mock := &helmSwapLLMMock{
		answerStarted: make(chan struct{}),
		answerDelay:   0,
	}
	tests := []struct {
		name, prompt, want string
	}{
		{
			name:   "source selection",
			prompt: "You select which declared RAG sources should be queried",
			want:   `{"names":["rag0"]}`,
		},
		{
			name:   "tier selection",
			prompt: "You select one chat-LLM word for a user's question",
			want:   `"tool":"invoke_llm_fast"`,
		},
		{
			name:   "answer",
			prompt: "You are a chatbot that answers a user's question",
			want:   "The mesh remained available",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]interface{}{
				"messages": []map[string]string{{
					"role": "system", "content": test.prompt,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/chat",
				strings.NewReader(string(payload)))
			response := httptest.NewRecorder()
			mock.serveChat(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			var body struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body.Message.Content, test.want) {
				t.Errorf("content = %q, want %q", body.Message.Content, test.want)
			}
		})
	}
	select {
	case <-mock.answerStarted:
	default:
		t.Fatal("answer request did not open the synchronization barrier")
	}
}
