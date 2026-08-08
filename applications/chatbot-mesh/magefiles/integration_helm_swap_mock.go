// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const helmSwapAnswerDelay = 30 * time.Second

// helmSwapLLMMock is a host-side Ollama boundary for the swap tracer. The first
// answer call stays active long enough for the replacement pod to become Ready
// and Kubernetes to begin terminating the old pod; a successful response then
// proves the old chatbot drained the machine_request instead of dropping it.
type helmSwapLLMMock struct {
	server        *http.Server
	listener      net.Listener
	answerStarted chan struct{}
	answerDelay   time.Duration
	delayOnce     sync.Once
}

func startHelmSwapLLMMock() (*helmSwapLLMMock, error) {
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("listen for helm-swap Ollama mock: %w", err)
	}
	mock := &helmSwapLLMMock{
		listener:      listener,
		answerStarted: make(chan struct{}),
		answerDelay:   helmSwapAnswerDelay,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", mock.serveTags)
	mux.HandleFunc("/api/embeddings", mock.serveEmbedding)
	mux.HandleFunc("/api/chat", mock.serveChat)
	mock.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = mock.server.Serve(listener) }()
	return mock, nil
}

func (m *helmSwapLLMMock) close() {
	_ = m.server.Close()
}

func (m *helmSwapLLMMock) helmArgs() []string {
	_, port, _ := net.SplitHostPort(m.listener.Addr().String())
	return []string{
		"--set", "llm.externalURL=http://host.docker.internal:" + port,
		"--set", "llm.port=" + port,
	}
}

func (m *helmSwapLLMMock) waitForAnswer(timeout time.Duration) error {
	select {
	case <-m.answerStarted:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("chat turn did not reach the delayed answer boundary within %s", timeout)
	}
}

func (m *helmSwapLLMMock) serveTags(w http.ResponseWriter, _ *http.Request) {
	writeHelmSwapJSON(w, map[string]any{"models": []map[string]string{
		{"name": "qwen3-embedding:8b"},
		{"name": "qwen2.5:3b"},
		{"name": "ornith:9b"},
	}})
}

func (m *helmSwapLLMMock) serveEmbedding(w http.ResponseWriter, _ *http.Request) {
	writeHelmSwapJSON(w, map[string]any{"embedding": []float64{0.11, 0.22, 0.33, 0.44}})
}

func (m *helmSwapLLMMock) serveChat(w http.ResponseWriter, request *http.Request) {
	var body struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var prompts strings.Builder
	for _, message := range body.Messages {
		prompts.WriteString(message.Content)
		prompts.WriteByte('\n')
	}
	content := "[tool_call]\n" +
		`{"tool":"invoke_llm_fast","parameters":{}}` +
		"\n[/tool_call]"
	switch text := prompts.String(); {
	case strings.Contains(text, "You select which declared RAG sources"):
		content = `{"names":["rag0"]}`
	case strings.Contains(text, "You are a chatbot that answers"):
		m.delayOnce.Do(func() {
			close(m.answerStarted)
			time.Sleep(m.answerDelay)
		})
		content = "The mesh remained available while its RAG topology changed."
	}
	writeHelmSwapJSON(w, map[string]any{
		"message":           map[string]string{"role": "assistant", "content": content},
		"eval_count":        4,
		"prompt_eval_count": 12,
	})
}

func writeHelmSwapJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
