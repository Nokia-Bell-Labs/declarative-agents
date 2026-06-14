// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/attribute"

	modelllm "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm/ollama"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/prompt"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/telemetry/genai"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
)

type invokeLLMCmd struct {
	client       modelllm.Client
	history      *modelllm.Conversation
	registry     *core.Registry
	assembler    modelllm.PromptAssembler
	state        core.State
	model        string
	providerName string
	serverAddr   string
	userMessage  string
	tracer       tracing.Tracer
	contextLimit int
	numCtx       int
	verbose      bool
	ctx          context.Context
	callTimeout  time.Duration
	prevLen      int
	prevMessages []modelllm.Message
	hasSnapshot  bool
}

func (c *invokeLLMCmd) Name() string { return "invoke_llm" }
func (c *invokeLLMCmd) SpanName() string {
	return genai.InferenceSpanName(c.model)
}
func (c *invokeLLMCmd) SpanCreationAttrs() []attribute.KeyValue {
	return genai.InferenceAttrs(c.providerName, c.model, c.serverAddr)
}

func (c *invokeLLMCmd) Execute() core.Result {
	c.ensureContext()
	c.snapshotHistory()
	messages := c.assemblePrompt()
	if res, ok := c.checkContextLimit(messages); ok {
		return res
	}
	chatResp, duration, err := c.chat(messages)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: err.Error(), Cost: core.Cost{Duration: duration}}
	}
	c.history.Append(modelllm.Message{Role: modelllm.Assistant, Content: chatResp.Content})
	return c.chatResult(chatResp, duration)
}

func (c *invokeLLMCmd) ensureContext() {
	if c.ctx == nil {
		c.ctx = context.Background()
	}
}

func (c *invokeLLMCmd) snapshotHistory() {
	c.prevLen = c.history.Len()
	c.prevMessages = c.history.Snapshot()
	c.hasSnapshot = true
	c.history.Append(modelllm.Message{Role: modelllm.User, Content: c.userMessage})
	c.tracer.Event("history.user_appended", attribute.Int("history_len", c.history.Len()))
}

func (c *invokeLLMCmd) assemblePrompt() []modelllm.Message {
	messages := c.assembler.AssembleMessages(c.history, c.registry, c.state)
	c.tracer.Event("prompt.assembled",
		attribute.Int("message_count", len(messages)),
		attribute.Int("history_messages", c.history.Len()),
	)
	return messages
}

func (c *invokeLLMCmd) checkContextLimit(messages []modelllm.Message) (core.Result, bool) {
	if c.contextLimit <= 0 {
		return core.Result{}, false
	}
	est := modelllm.EstimateTokens(messages)
	c.tracer.SetAttributes(attribute.Int("context.estimated_tokens", est), attribute.Int("context.limit", c.contextLimit))
	if est <= c.contextLimit {
		return core.Result{}, false
	}
	err := fmt.Errorf("context window exhaustion: estimated %d tokens exceeds limit %d", est, c.contextLimit)
	output := fmt.Sprintf("assembled prompt (%d estimated tokens) does not fit in context window (%d)", est, c.contextLimit)
	return core.Result{Signal: core.CommandError, Err: err, Output: output}, true
}

func (c *invokeLLMCmd) chat(messages []modelllm.Message) (modelllm.ChatResponse, time.Duration, error) {
	opts := modelllm.ChatOptions{Model: c.model, Temperature: 0, Seed: 42, NumCtx: c.numCtx}
	if c.verbose {
		if inputJSON, err := json.Marshal(messages); err == nil {
			c.tracer.SetAttributes(genai.AttrInputMessages.String(string(inputJSON)))
		}
	}
	chatCtx, cancel := c.chatContext()
	defer cancel()
	c.tracer.Event("chat.request_start")
	start := time.Now()
	chatResp, err := c.client.Chat(chatCtx, messages, opts)
	return chatResp, time.Since(start), err
}

func (c *invokeLLMCmd) chatContext() (context.Context, context.CancelFunc) {
	if c.callTimeout <= 0 {
		return c.ctx, func() {}
	}
	return context.WithTimeout(c.ctx, c.callTimeout)
}

func (c *invokeLLMCmd) chatResult(chatResp modelllm.ChatResponse, duration time.Duration) core.Result {
	c.tracer.Event("chat.request_done", attribute.Int("response_content_len", len(chatResp.Content)))
	c.tracer.Event("history.assistant_appended", attribute.Int("history_len", c.history.Len()))
	cost := core.Cost{Duration: duration, TokensIn: chatResp.TokensIn, TokensOut: chatResp.TokensOut}
	c.tracer.SetAttributes(genai.AttrUsageInputTokens.Int(cost.TokensIn), genai.AttrUsageOutputTokens.Int(cost.TokensOut))
	if c.verbose {
		c.tracer.SetAttributes(genai.AttrOutputMessages.String(chatResp.Content))
	}
	return core.Result{Signal: core.LLMResponded, Output: chatResp.Content, Cost: cost}
}

// InvokeLLMBuilder constructs invoke_llm commands.
type InvokeLLMBuilder struct {
	Client       modelllm.Client
	History      *modelllm.Conversation
	Registry     *core.Registry
	Assembler    modelllm.PromptAssembler
	State        core.State
	Model        string
	ProviderName string
	ServerAddr   string
	Tracer       tracing.Tracer
	ContextLimit int
	NumCtx       int
	CallTimeout  time.Duration
	Verbose      bool
	Ctx          context.Context
}

// InvokeLLMFactoryDeps are process-local ports for invoke_llm construction.
type InvokeLLMFactoryDeps struct {
	History     *modelllm.Conversation
	Registry    *core.Registry
	Tracer      tracing.Tracer
	ProfilesDir string
	Verbose     bool
	Ctx         context.Context
	OnResolved  func(InvokeLLMResolvedConfig)
}

// InvokeLLMResolvedConfig exposes metadata needed by neighboring tools.
type InvokeLLMResolvedConfig struct {
	Model        string
	ProviderName string
	Parser       modelllm.ResponseParser
	MaxTime      time.Duration
	MaxTokens    int
}

// NewInvokeLLMBuilder creates the configured invoke_llm builder.
func NewInvokeLLMBuilder(def catalog.ToolDef, deps InvokeLLMFactoryDeps) (*InvokeLLMBuilder, error) {
	cfg, err := DecodeInvokeLLMConfig(def)
	if err != nil {
		return nil, err
	}
	parser, err := resolveLLMParser(cfg.Model, deps.ProfilesDir)
	if err != nil {
		return nil, err
	}
	client, serverAddr, err := newLLMClient(cfg, deps.Tracer)
	if err != nil {
		return nil, err
	}
	if deps.OnResolved != nil {
		deps.OnResolved(resolvedLLMConfig(cfg, parser))
	}
	return invokeBuilder(cfg, parser, client, serverAddr, deps), nil
}

func invokeBuilder(cfg catalog.LLMToolConfig, parser modelllm.ResponseParser, client modelllm.Client, serverAddr string, deps InvokeLLMFactoryDeps) *InvokeLLMBuilder {
	return &InvokeLLMBuilder{
		Client: client, History: deps.History, Registry: deps.Registry,
		Assembler: newLLMAssembler(cfg, parser), State: core.State(cfg.ManifestState),
		Model: cfg.Model, ProviderName: cfg.Provider, ServerAddr: serverAddr,
		Tracer: deps.Tracer, NumCtx: cfg.NumCtx, CallTimeout: durationSeconds(cfg.LLMTimeout),
		Verbose: deps.Verbose, Ctx: deps.Ctx,
	}
}

func (b *InvokeLLMBuilder) Build(res core.Result) core.Command {
	ctx := b.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return &invokeLLMCmd{
		client: b.Client, history: b.History, registry: b.Registry, assembler: b.Assembler,
		state: b.State, model: b.Model, providerName: b.ProviderName, serverAddr: b.ServerAddr,
		userMessage: res.Output, tracer: b.Tracer, contextLimit: b.ContextLimit,
		numCtx: b.NumCtx, callTimeout: b.CallTimeout, verbose: b.Verbose, ctx: ctx,
	}
}

func newLLMAssembler(cfg catalog.LLMToolConfig, parser modelllm.ResponseParser) modelllm.PromptAssembler {
	return &modelllm.DefaultAssembler{Prompt: prompt.Prompt{Role: cfg.SystemPrompt, OutputFormat: cfg.ToolPrompt}, Parser: parser}
}

func newLLMClient(cfg catalog.LLMToolConfig, tracer tracing.Tracer) (modelllm.Client, string, error) {
	if cfg.Provider != "ollama" {
		return nil, "", fmt.Errorf("unsupported invoke_llm provider %q", cfg.Provider)
	}
	if cfg.ProviderURL == "" {
		return nil, "", fmt.Errorf("invoke_llm config provider %q requires provider_url", cfg.Provider)
	}
	client, err := ollama.NewAdapter(cfg.ProviderURL, cfg.Model,
		ollama.WithHTTPClient(&http.Client{Timeout: httpTimeout(cfg)}),
		ollama.WithTracer(tracer),
	)
	return client, serverAddr(cfg.ProviderURL), err
}

func resolveLLMParser(model, profilesDir string) (modelllm.ResponseParser, error) {
	reg, err := modelllm.DefaultProfileRegistry()
	if profilesDir != "" {
		reg, err = modelllm.LoadProfiles(profilesDir)
	}
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	return reg.ResolveProfile(model), nil
}

func resolvedLLMConfig(cfg catalog.LLMToolConfig, parser modelllm.ResponseParser) InvokeLLMResolvedConfig {
	return InvokeLLMResolvedConfig{
		Model: cfg.Model, ProviderName: cfg.Provider, Parser: parser,
		MaxTime: durationSeconds(cfg.MaxTime), MaxTokens: cfg.MaxTokens,
	}
}

func httpTimeout(cfg catalog.LLMToolConfig) time.Duration {
	timeout := 5 * time.Minute
	if maxTime := durationSeconds(cfg.MaxTime); maxTime > timeout {
		timeout = maxTime
	}
	return timeout
}

func durationSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func serverAddr(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}
