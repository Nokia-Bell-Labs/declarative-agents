// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm/ollama"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/prompt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// Deterministic decoding defaults applied when invoke_llm config omits the
// temperature and seed fields, preserving the agent's reproducible baseline.
const (
	defaultTemperature = 0.0
	defaultSeed        = 42
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
	promptFrom   string
	view         core.CommandStateView
	tracer       tracing.Tracer
	contextLimit int
	numCtx       int
	temperature  float64
	seed         int
	verbose      bool
	ctx          context.Context
	callTimeout  time.Duration
	metrics      core.MetricConfig
	recorder     monitor.ToolMetricsRecorder
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

// SetCommandState receives the read-only command-state view the engine injects
// before dispatch, so a configured user_prompt_from selector resolves against
// prior steps (core.CommandStateAware).
func (c *invokeLLMCmd) SetCommandState(view core.CommandStateView) { c.view = view }

var _ core.CommandStateAware = (*invokeLLMCmd)(nil)

func (c *invokeLLMCmd) Execute() core.Result {
	c.ensureContext()
	c.resolveUserMessage()
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

// resolveUserMessage overrides the user message with the value the configured
// user_prompt_from selector resolves to in command state. It runs before the
// history snapshot so the resolved prompt is the turn's user message. An empty
// selector or an unresolved one leaves the dispatch Result's Output in place, so
// a $tool-dispatched word degrades to its dispatch input rather than failing.
//
// Two selector forms are supported: "$from(label)" with no path takes the raw
// Output of the labeled step (a compose word renders a plain prompt string, not a
// JSON object, so a path selector cannot address it); "$from(label).dotted.path"
// walks the labeled step's JSON output.
func (c *invokeLLMCmd) resolveUserMessage() {
	if c.promptFrom == "" || c.view == nil {
		return
	}
	if label, ok := wholeOutputSelector(c.promptFrom); ok {
		output, found := c.view.Lookup(label)
		if !found {
			c.tracer.Event("user_prompt.unresolved",
				attribute.String("selector", c.promptFrom),
				attribute.String("error", "no prior step labeled "+label))
			return
		}
		c.userMessage = output
		c.tracer.Event("user_prompt.from_command_state", attribute.String("selector", c.promptFrom))
		return
	}
	value, err := core.ResolveFromSelector(c.view, c.promptFrom)
	if err != nil {
		c.tracer.Event("user_prompt.unresolved",
			attribute.String("selector", c.promptFrom),
			attribute.String("error", err.Error()))
		return
	}
	c.userMessage = stringifyPrompt(value)
	c.tracer.Event("user_prompt.from_command_state", attribute.String("selector", c.promptFrom))
}

// wholeOutputSelector matches "$from(label)" with no dotted path and returns the
// label; the step's raw Output becomes the user message.
func wholeOutputSelector(selector string) (string, bool) {
	const prefix = "$from("
	if !strings.HasPrefix(selector, prefix) || !strings.HasSuffix(selector, ")") {
		return "", false
	}
	label := selector[len(prefix) : len(selector)-1]
	if label == "" || strings.ContainsAny(label, "().") {
		return "", false
	}
	return label, true
}

// stringifyPrompt renders a command-state value as the user message: strings
// pass through; anything else is JSON-encoded so structured values read
// predictably.
func stringifyPrompt(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
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
	opts := modelllm.ChatOptions{Model: c.model, Temperature: c.temperature, Seed: c.seed, NumCtx: c.numCtx}
	c.tracer.SetAttributes(
		genai.AttrRequestTemperature.Float64(c.temperature),
		genai.AttrRequestSeed.Int(c.seed),
	)
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
	c.recordTokenMetrics(cost)
	return core.Result{
		Signal:  core.LLMResponded,
		Output:  chatResp.Content,
		Cost:    cost,
		Receipt: encodeConversationReceipt(c.prevMessages),
	}
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
	Temperature  float64
	Seed         int
	CallTimeout  time.Duration
	Metrics      core.MetricConfig
	Verbose      bool
	Ctx          context.Context
	// UserPromptFrom, when set, is the command-state $from selector the built
	// command resolves its user message from instead of the dispatch Result.
	UserPromptFrom string
}

// InvokeLLMFactoryDeps are process-local ports for invoke_llm construction.
type InvokeLLMFactoryDeps struct {
	History    *modelllm.Conversation
	Registry   *core.Registry
	Tracer     tracing.Tracer
	Verbose    bool
	Ctx        context.Context
	OnResolved func(InvokeLLMResolvedConfig)
}

// InvokeLLMResolvedConfig exposes metadata needed by neighboring tools.
type InvokeLLMResolvedConfig struct {
	Model         string
	ProviderName  string
	Parser        modelllm.ResponseParser
	ManifestState core.State
	MaxTime       time.Duration
	MaxTokens     int
}

// NewInvokeLLMBuilder creates the configured invoke_llm builder.
func NewInvokeLLMBuilder(def catalog.ToolDef, deps InvokeLLMFactoryDeps) (*InvokeLLMBuilder, error) {
	cfg, err := DecodeInvokeLLMConfig(def)
	if err != nil {
		return nil, err
	}
	parser, err := resolveLLMParser(cfg)
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
	return invokeBuilder(def, cfg, parser, client, serverAddr, deps), nil
}

func invokeBuilder(
	def catalog.ToolDef,
	cfg catalog.LLMToolConfig,
	parser modelllm.ResponseParser,
	client modelllm.Client,
	serverAddr string,
	deps InvokeLLMFactoryDeps,
) *InvokeLLMBuilder {
	return &InvokeLLMBuilder{
		Client: client, History: deps.History, Registry: deps.Registry,
		Assembler: newLLMAssembler(cfg, parser), State: core.State(cfg.ManifestState),
		Model: cfg.Model, ProviderName: cfg.Provider, ServerAddr: serverAddr,
		Tracer: deps.Tracer, NumCtx: cfg.NumCtx,
		Temperature: resolveTemperature(cfg), Seed: resolveSeed(cfg),
		CallTimeout: durationSeconds(cfg.LLMTimeout),
		Metrics:     def.Metrics, Verbose: deps.Verbose, Ctx: deps.Ctx,
		UserPromptFrom: cfg.UserPromptFrom,
	}
}

// resolveTemperature returns the configured temperature or the deterministic
// default when the invoke_llm config omits the field.
func resolveTemperature(cfg catalog.LLMToolConfig) float64 {
	if cfg.Temperature != nil {
		return *cfg.Temperature
	}
	return defaultTemperature
}

// resolveSeed returns the configured seed or the deterministic default when the
// invoke_llm config omits the field.
func resolveSeed(cfg catalog.LLMToolConfig) int {
	if cfg.Seed != nil {
		return *cfg.Seed
	}
	return defaultSeed
}

func (b *InvokeLLMBuilder) Build(res core.Result) core.Command {
	ctx := b.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	state := res.State
	if state == "" {
		state = b.State
	}
	return &invokeLLMCmd{
		client: b.Client, history: b.History, registry: b.Registry, assembler: b.Assembler,
		state: state, model: b.Model, providerName: b.ProviderName, serverAddr: b.ServerAddr,
		userMessage: res.Output, promptFrom: b.UserPromptFrom, tracer: b.Tracer, contextLimit: b.ContextLimit,
		numCtx: b.NumCtx, temperature: b.Temperature, seed: b.Seed,
		callTimeout: b.CallTimeout, metrics: b.Metrics, verbose: b.Verbose, ctx: ctx,
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

func resolveLLMParser(cfg catalog.LLMToolConfig) (modelllm.ResponseParser, error) {
	reg, err := modelllm.DefaultProfileRegistry()
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	if cfg.ResponseProfile == "" {
		return reg.ResolveProfile(cfg.Model), nil
	}
	parser, ok := reg.ResolveProfileName(cfg.ResponseProfile)
	if !ok {
		return nil, fmt.Errorf("invoke_llm response_profile %q not found", cfg.ResponseProfile)
	}
	return parser, nil
}

func resolvedLLMConfig(cfg catalog.LLMToolConfig, parser modelllm.ResponseParser) InvokeLLMResolvedConfig {
	return InvokeLLMResolvedConfig{
		Model: cfg.Model, ProviderName: cfg.Provider, Parser: parser, ManifestState: core.State(cfg.ManifestState),
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
