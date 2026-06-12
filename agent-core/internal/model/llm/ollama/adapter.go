// Copyright (c) 2026 Nokia. All rights reserved.

// Package ollama implements the llm.Client interface for the Ollama
// inference server. It communicates via the Ollama HTTP API: POST
// /api/chat for completions and GET /api/tags for model discovery.
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

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/telemetry/genai"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
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

// tagsResp is the minimal GET /api/tags response used by checkModel.
type tagsResp struct {
	Models []modelEntry `json:"models"`
}

type modelEntry struct {
	Name string `json:"name"`
}

// detailedTagsResp is the full GET /api/tags response with metadata.
type detailedTagsResp struct {
	Models []detailedModelEntry `json:"models"`
}

type detailedModelEntry struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
	Digest     string    `json:"digest"`
	Details    struct {
		ParameterSize     string `json:"parameter_size"`
		QuantizationLevel string `json:"quantization_level"`
		Family            string `json:"family"`
	} `json:"details"`
}

// Adapter wraps the Ollama HTTP API and implements llm.Client.
type Adapter struct {
	baseURL        string
	model          string
	client         *http.Client
	tracer         tracing.Tracer
	skipModelCheck bool
}

var _ llm.Client = (*Adapter)(nil)

// NewAdapter creates an Adapter and verifies that the requested model
// is available via GET /api/tags. Returns an error if the model is not
// found or the connection fails.
func NewAdapter(baseURL, model string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
	for _, opt := range opts {
		opt(a)
	}

	if !a.skipModelCheck {
		if err := a.checkModel(); err != nil {
			return nil, err
		}
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
	defer resp.Body.Close()

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

	tr.SetAttributes(
		genai.AttrUsageInputTokens.Int(cr.PromptEvalCount),
		genai.AttrUsageOutputTokens.Int(cr.EvalCount),
		genai.AttrResponseModel.String(opts.Model),
	)

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
		fmt.Sscanf(u.Port(), "%d", &port)
		if port > 0 {
			attrs = append(attrs, genai.AttrServerPort.Int(port))
		}
	}

	return a.tracer.Push(genai.InferenceSpanName(model), attrs...)
}

// ListModels queries Ollama GET /api/tags and returns metadata for all
// locally available models. Satisfies llm.Client.
func (a *Adapter) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	a.traceEvent("list_models.start",
		attribute.String("ollama.base_url", a.baseURL),
	)

	url := a.baseURL + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		a.traceEvent("list_models.error", attribute.String("error", err.Error()))
		return nil, fmt.Errorf("failed to connect to Ollama at %s: %w", a.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		a.traceEvent("list_models.error", attribute.Int("http.status_code", resp.StatusCode))
		return nil, fmt.Errorf("ollama /api/tags returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read /api/tags response: %w", err)
	}

	var tags detailedTagsResp
	if err := json.Unmarshal(body, &tags); err != nil {
		return nil, fmt.Errorf("parse /api/tags response: %w", err)
	}

	models := make([]llm.ModelInfo, len(tags.Models))
	for i, m := range tags.Models {
		details := map[string]string{
			"digest": m.Digest,
		}
		if m.Details.ParameterSize != "" {
			details["parameter_size"] = m.Details.ParameterSize
		}
		if m.Details.QuantizationLevel != "" {
			details["quantization_level"] = m.Details.QuantizationLevel
		}
		if m.Details.Family != "" {
			details["family"] = m.Details.Family
		}
		models[i] = llm.ModelInfo{
			Name:       m.Name,
			Size:       m.Size,
			ModifiedAt: m.ModifiedAt,
			Provider:   "ollama",
			Details:    details,
		}
	}

	a.traceEvent("list_models.done", attribute.Int("model_count", len(models)))
	return models, nil
}

// checkModel verifies that a.model is available via GET /api/tags.
func (a *Adapter) checkModel() error {
	a.traceEvent("check_model.start",
		attribute.String("ollama.base_url", a.baseURL),
		attribute.String("llm.model", a.model),
	)

	resp, err := a.client.Get(a.baseURL + "/api/tags")
	if err != nil {
		a.traceEvent("check_model.error", attribute.String("error", err.Error()))
		return fmt.Errorf("failed to connect to Ollama at %s: %w", a.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		a.traceEvent("check_model.error", attribute.Int("http.status_code", resp.StatusCode))
		return fmt.Errorf("ollama /api/tags returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read /api/tags response: %w", err)
	}

	var tags tagsResp
	if err := json.Unmarshal(body, &tags); err != nil {
		return fmt.Errorf("parse /api/tags response: %w", err)
	}

	matched := matchModel(a.model, tags.Models)
	a.traceEvent("check_model.done",
		attribute.Int("model_count", len(tags.Models)),
		attribute.Bool("match", matched),
	)

	if matched {
		return nil
	}
	return fmt.Errorf("model %q is not available locally; run \"ollama pull %s\" and retry", a.model, a.model)
}

func (a *Adapter) traceEvent(name string, attrs ...attribute.KeyValue) {
	if a.tracer != nil {
		a.tracer.Event(name, attrs...)
	}
}

// matchModel checks whether name matches any entry. Case-insensitive.
// If name omits a tag (no ":"), it matches entries with any tag.
func matchModel(name string, models []modelEntry) bool {
	lower := strings.ToLower(name)
	hasTag := strings.Contains(lower, ":")

	for _, m := range models {
		entry := strings.ToLower(m.Name)
		if hasTag {
			if entry == lower {
				return true
			}
		} else {
			bare := strings.SplitN(entry, ":", 2)[0]
			if bare == lower {
				return true
			}
		}
	}
	return false
}
