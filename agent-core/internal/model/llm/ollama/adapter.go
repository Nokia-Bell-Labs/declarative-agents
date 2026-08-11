// Copyright (c) 2026 Nokia. All rights reserved.

// Package ollama implements the llm.Client interface for the Ollama
// inference server through POST /api/chat.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

// chatReq is the JSON body sent to Ollama POST /api/chat.
type chatReq struct {
	Model    string   `json:"model"`
	Messages []msgDTO `json:"messages"`
	Stream   bool     `json:"stream"`
	Options  chatOpts `json:"options"`
}

type msgDTO struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatOpts struct {
	Temperature float64 `json:"temperature"`
	Seed        int     `json:"seed"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

// chatResp is the JSON body returned from Ollama POST /api/chat.
type chatResp struct {
	Message         msgDTO `json:"message"`
	EvalCount       int    `json:"eval_count"`
	PromptEvalCount int    `json:"prompt_eval_count"`
}

// Adapter wraps the Ollama HTTP API and implements llm.Client.
type Adapter struct {
	baseURL string
	model   string
	client  *http.Client
	tracer  tracing.Tracer
}

var _ llm.Client = (*Adapter)(nil)

// NewAdapter creates an Adapter. Availability checks are explicit profile
// words rather than hidden constructor I/O.
func NewAdapter(baseURL, model string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
	for _, opt := range opts {
		opt(a)
	}

	return a, nil
}

// Model returns the model name this adapter was created for.
func (a *Adapter) Model() string { return a.model }

// Chat sends a chat request to Ollama POST /api/chat and returns the
// response. Satisfies llm.Client.
func (a *Adapter) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (llm.ChatResponse, error) {
	tr, span := a.chatSpan(ctx, opts.Model)
	defer span()

	dtos := make([]msgDTO, len(messages))
	for i, m := range messages {
		dtos[i] = msgDTO{Role: string(m.Role), Content: m.Content}
	}

	req := chatReq{
		Model:    opts.Model,
		Messages: dtos,
		Stream:   false,
		Options:  chatOpts{Temperature: opts.Temperature, Seed: opts.Seed, NumCtx: opts.NumCtx},
	}

	body, err := json.Marshal(req)
	if err != nil {
		tr.RecordError(fmt.Errorf("marshal chat request: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		tr.RecordError(fmt.Errorf("create HTTP request: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		tr.RecordError(fmt.Errorf("ollama chat request failed: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("ollama chat request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("ollama /api/chat returned status %d: %s", resp.StatusCode, string(respBody))
		tr.SetAttributes(genai.AttrErrorType.String(fmt.Sprintf("%d", resp.StatusCode)))
		tr.RecordError(err)
		return llm.ChatResponse{}, err
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		tr.RecordError(fmt.Errorf("read chat response: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("read chat response: %w", err)
	}

	var cr chatResp
	if err := json.Unmarshal(respBody, &cr); err != nil {
		tr.RecordError(fmt.Errorf("parse chat response: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("parse chat response: %w", err)
	}

	usageAttrs := genai.UsageAttrs(cr.PromptEvalCount, cr.EvalCount)
	tr.SetAttributes(append(usageAttrs, genai.AttrResponseModel.String(opts.Model))...)

	return llm.ChatResponse{
		Content:   cr.Message.Content,
		TokensIn:  cr.PromptEvalCount,
		TokensOut: cr.EvalCount,
	}, nil
}

// chatSpan creates a semconv inference span for the Chat call if a
// tracer is configured, otherwise returns a noop.
func (a *Adapter) chatSpan(ctx context.Context, model string) (tracing.Tracer, func()) {
	if a.tracer == nil {
		return tracing.NoopTracer{}, func() {}
	}

	serverAddr := ""
	if u, err := url.Parse(a.baseURL); err == nil {
		serverAddr = u.Host
	}

	attrs := genai.InferenceAttrs(genai.ProviderOllama, model, serverAddr)
	if u, err := url.Parse(a.baseURL); err == nil && u.Port() != "" {
		port := 0
		n, err := fmt.Sscanf(u.Port(), "%d", &port)
		if err == nil && n == 1 && port > 0 {
			attrs = append(attrs, genai.AttrServerPort.Int(port))
		}
	}

	return a.tracer.Push(genai.InferenceSpanName(model), attrs...)
}
