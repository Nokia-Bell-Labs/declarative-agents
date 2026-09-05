// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package cohere

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

const (
	maxResponseBytes   = 1 << 20
	maxDiagnosticBytes = 4 << 10
)

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature"`
	Seed        int       `json:"seed"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message struct {
		Content []contentBlock `json:"content"`
	} `json:"message"`
	Usage struct {
		Tokens struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
		} `json:"tokens"`
	} `json:"usage"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type providerError struct {
	kind    string
	message string
	cause   error
}

func (e *providerError) Error() string { return e.message }
func (e *providerError) Unwrap() error { return e.cause }

func newProviderError(kind, format string, args ...interface{}) error {
	return &providerError{kind: kind, message: fmt.Sprintf(format, args...)}
}

func wrapProviderError(kind, message string, cause error) error {
	return &providerError{kind: kind, message: message + ": " + cause.Error(), cause: cause}
}

// Chat sends one Cohere v2 request and normalizes its response.
func (a *Adapter) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (llm.ChatResponse, error) {
	ctx, tracer, done := a.chatSpan(ctx, opts.Model)
	defer done()
	key, err := a.resolveAPIKey()
	if err != nil {
		return llm.ChatResponse{}, recordFailure(tracer, err)
	}
	request, err := a.newChatRequest(ctx, key, messages, opts)
	if err != nil {
		return llm.ChatResponse{}, recordFailure(tracer, err)
	}
	response, err := a.send(request, key)
	if err != nil {
		return llm.ChatResponse{}, recordFailure(tracer, err)
	}
	tracer.SetAttributes(append(
		genai.UsageAttrs(response.TokensIn, response.TokensOut),
		genai.AttrResponseModel.String(opts.Model),
	)...)
	return response, nil
}

func (a *Adapter) resolveAPIKey() (string, error) {
	if a.apiKey == nil {
		return "", newProviderError("credential", "cohere credential lookup is not configured")
	}
	key, ok := a.apiKey()
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return "", newProviderError("credential", "cohere credential %s is not resolved", apiKeyEnvironment)
	}
	return key, nil
}

func (a *Adapter) newChatRequest(
	ctx context.Context,
	key string,
	messages []llm.Message,
	opts llm.ChatOptions,
) (*http.Request, error) {
	body, err := json.Marshal(makeChatRequest(messages, opts))
	if err != nil {
		return nil, wrapProviderError("request_encode", "marshal Cohere chat request", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v2/chat", bytes.NewReader(body))
	if err != nil {
		return nil, wrapProviderError("request_create", "create Cohere chat request", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	return request, nil
}

func makeChatRequest(messages []llm.Message, opts llm.ChatOptions) chatRequest {
	requestMessages := make([]message, len(messages))
	for i, item := range messages {
		requestMessages[i] = message{Role: string(item.Role), Content: item.Content}
	}
	return chatRequest{
		Model: opts.Model, Messages: requestMessages, Stream: false,
		Temperature: opts.Temperature, Seed: opts.Seed,
	}
}

func (a *Adapter) send(request *http.Request, key string) (llm.ChatResponse, error) {
	response, err := a.client.Do(request)
	if err != nil {
		return llm.ChatResponse{}, wrapProviderError("connection", "cohere chat request failed", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := readBounded(response.Body, maxResponseBytes)
	if err != nil {
		return llm.ChatResponse{}, wrapProviderError("response_read", "read Cohere chat response", err)
	}
	if response.StatusCode != http.StatusOK {
		diagnostic := safeDiagnostic(body, key)
		return llm.ChatResponse{}, newProviderError(
			fmt.Sprintf("%d", response.StatusCode),
			"cohere /v2/chat returned status %d: %s", response.StatusCode, diagnostic,
		)
	}
	return decodeChatResponse(body)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return body, nil
}

func safeDiagnostic(body []byte, key string) string {
	if len(body) > maxDiagnosticBytes {
		body = body[:maxDiagnosticBytes]
	}
	value := strings.TrimSpace(string(body))
	if key != "" {
		value = strings.ReplaceAll(value, key, "[REDACTED]")
	}
	return value
}

func decodeChatResponse(body []byte) (llm.ChatResponse, error) {
	var response chatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return llm.ChatResponse{}, wrapProviderError("response_decode", "parse Cohere chat response", err)
	}
	content := joinTextBlocks(response.Message.Content)
	if content == "" {
		return llm.ChatResponse{}, newProviderError("response_content", "Cohere chat response contains no text content")
	}
	return llm.ChatResponse{
		Content: content, TokensIn: response.Usage.Tokens.Input, TokensOut: response.Usage.Tokens.Output,
	}, nil
}

func joinTextBlocks(blocks []contentBlock) string {
	var content strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}
	return content.String()
}

func recordFailure(tracer tracing.Tracer, err error) error {
	var providerErr *providerError
	kind := "_OTHER"
	if errors.As(err, &providerErr) {
		kind = providerErr.kind
	}
	tracer.SetAttributes(genai.AttrErrorType.String(kind))
	tracer.RecordError(err)
	return err
}
