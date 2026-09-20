// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm/dialect"
	restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

const (
	dialectInstrumentation  = "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm/dialect"
	maxDialectResponseBytes = 1 << 20
	maxDialectDiagnostic    = 4 << 10
)

// dialectClient is invoke_llm's provider step as a template method: the
// request body, the reply selectors, the failure map, and the auth profile
// come from the bound chat dialect, and nothing here names a provider
// (srd058 R3.1).
type dialectClient struct {
	chat        dialect.Chat
	baseURL     string
	http        *http.Client
	tracer      tracing.Tracer
	credentials credentials.Resolver
}

var _ modelllm.Client = (*dialectClient)(nil)

func newDialectClient(chat dialect.Chat, baseURL string, httpClient *http.Client,
	tracer tracing.Tracer, resolver credentials.Resolver,
) *dialectClient {
	if resolver == nil {
		resolver = credentials.Environment{}
	}
	return &dialectClient{
		chat: chat, baseURL: strings.TrimRight(baseURL, "/"), http: httpClient,
		tracer: tracerOrNoop(tracer), credentials: resolver,
	}
}

// providerError carries the span's error.type: the HTTP status for a provider
// answer, or a short kind for a failure before or after the exchange. signal
// is the shared failure vocabulary the dialect maps that answer to, empty for
// a fault that does not describe the provider's state (srd058 R2.4).
type providerError struct {
	kind    string
	message string
	signal  string
}

func (e *providerError) Error() string { return e.message }

// ProviderFailureSignal returns the failure signal err describes, or empty
// when the error is not a classified provider failure. A caller that opted
// into the taxonomy emits it in place of CommandError.
func ProviderFailureSignal(err error) string {
	var failure *providerError
	if errors.As(err, &failure) {
		return failure.signal
	}
	return ""
}

// Chat builds, sends, and reads one chat request.
func (c *dialectClient) Chat(ctx context.Context, messages []modelllm.Message, opts modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	ctx, tracer, done := tracing.ContextChild(ctx, c.tracer, dialectInstrumentation,
		genai.InferenceSpanName(opts.Model), c.spanAttrs(opts.Model)...)
	defer done()
	request, err := c.request(ctx, messages, opts)
	if err != nil {
		return modelllm.ChatResponse{}, recordDialectFailure(tracer, err)
	}
	reply, err := c.send(request)
	if err != nil {
		return modelllm.ChatResponse{}, recordDialectFailure(tracer, err)
	}
	tracer.SetAttributes(append(
		genai.UsageAttrs(reply.TokensIn, reply.TokensOut),
		genai.AttrResponseModel.String(opts.Model),
	)...)
	return modelllm.ChatResponse{Content: reply.Text, TokensIn: reply.TokensIn, TokensOut: reply.TokensOut}, nil
}

func (c *dialectClient) spanAttrs(model string) []attribute.KeyValue {
	attrs := genai.InferenceAttrs(c.chat.ProviderName, model, serverAddr(c.baseURL))
	if parsed, err := url.Parse(c.baseURL); err == nil && parsed.Port() != "" {
		if port, err := strconv.Atoi(parsed.Port()); err == nil && port > 0 {
			attrs = append(attrs, genai.AttrServerPort.Int(port))
		}
	}
	return attrs
}

// request renders the body and authenticates the request. The credential is
// resolved here, immediately before the call, and a missing one fails before
// any network I/O (srd058 R3.5).
func (c *dialectClient) request(ctx context.Context, messages []modelllm.Message, opts modelllm.ChatOptions) (*http.Request, error) {
	body, err := c.chat.RenderBody(dialectParams(messages, opts))
	if err != nil {
		return nil, &providerError{kind: "request_encode", message: err.Error()}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, &providerError{kind: "request_encode", message: "encode chat request: " + err.Error()}
	}
	request, err := http.NewRequestWithContext(ctx, c.chat.Method, c.baseURL+c.chat.Path, bytes.NewReader(encoded))
	if err != nil {
		return nil, &providerError{kind: "request_create", message: "create chat request: " + err.Error()}
	}
	request.Header.Set("Content-Type", "application/json")
	if err := restclient.ApplyAuth(request, c.chat.Auth, c.credentials); err != nil {
		return nil, &providerError{kind: "credential", message: fmt.Sprintf("%s chat: %v", c.chat.ProviderName, err)}
	}
	return request, nil
}

// dialectParams are the params a body template may reference. An option the
// config leaves unset is absent, so the dialect omits it (srd058 R2.2).
func dialectParams(messages []modelllm.Message, opts modelllm.ChatOptions) map[string]interface{} {
	encoded := make([]interface{}, len(messages))
	for i, message := range messages {
		encoded[i] = map[string]interface{}{"role": string(message.Role), "content": message.Content}
	}
	params := map[string]interface{}{
		dialect.ParamMessages: encoded, dialect.ParamModel: opts.Model,
		dialect.ParamTemperature: opts.Temperature, dialect.ParamSeed: opts.Seed,
	}
	if opts.NumCtx > 0 {
		params[dialect.ParamNumCtx] = opts.NumCtx
	}
	return params
}

func (c *dialectClient) send(request *http.Request) (dialect.Reply, error) {
	response, err := c.http.Do(request)
	if err != nil {
		// A refused connection, a timeout, and a cancelled context carry no
		// status to map, and all three say the same thing: the provider did
		// not answer (srd058 R2.4).
		return dialect.Reply{}, &providerError{kind: "connection",
			message: fmt.Sprintf("%s chat request failed: %v", c.chat.ProviderName, err),
			signal:  dialect.ProviderUnavailable}
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDialectResponseBytes+1))
	if err != nil || len(body) > maxDialectResponseBytes {
		return dialect.Reply{}, &providerError{kind: "response_read",
			message: fmt.Sprintf("read %s chat response: exceeds %d bytes or failed", c.chat.ProviderName, maxDialectResponseBytes)}
	}
	if response.StatusCode != http.StatusOK {
		return dialect.Reply{}, c.statusError(response.StatusCode, body)
	}
	reply, err := c.chat.ExtractReply(body)
	if errors.Is(err, dialect.ErrNoText) {
		return dialect.Reply{}, &providerError{kind: "response_content", message: err.Error()}
	}
	if err != nil {
		return dialect.Reply{}, &providerError{kind: "response_decode", message: err.Error()}
	}
	return reply, nil
}

// statusError reports a provider's error answer with the failure signal the
// dialect maps it to. The body is truncated and every credential the auth
// profile names is redacted before it reaches an error, a log, or a span.
func (c *dialectClient) statusError(status int, body []byte) error {
	if len(body) > maxDialectDiagnostic {
		body = body[:maxDialectDiagnostic]
	}
	diagnostic := strings.TrimSpace(string(body))
	auth := c.chat.Auth
	for _, ref := range []string{auth.TokenRef, auth.UsernameRef, auth.PasswordRef} {
		if secret, err := credentials.Resolve(c.credentials, ref); err == nil && secret != "" {
			diagnostic = strings.ReplaceAll(diagnostic, secret, "[REDACTED]")
		}
	}
	signal := c.chat.FailureSignal(status)
	described := signal
	if described == "" {
		described = "unmapped"
	}
	return &providerError{kind: strconv.Itoa(status), signal: signal, message: fmt.Sprintf(
		"%s %s returned status %d (%s): %s", c.chat.ProviderName, c.chat.Path, status, described, diagnostic)}
}

func recordDialectFailure(tracer tracing.Tracer, err error) error {
	kind := "_OTHER"
	if failure, ok := err.(*providerError); ok {
		kind = failure.kind
	}
	tracer.SetAttributes(genai.AttrErrorType.String(kind))
	tracer.RecordError(err)
	return err
}
