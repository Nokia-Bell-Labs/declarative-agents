// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// InitRelaySpans identifies the declared OTLP batch relay factory.
	InitRelaySpans      = "relay_spans"
	defaultRelayTimeout = 10 * time.Second
)

// RelayConfig configures one trusted upstream trace export.
type RelayConfig struct {
	Endpoint        string
	ReceiverAddress string
	BatchSource     string
	Timeout         time.Duration
}

// RelayBuilder constructs upstream trace relay commands.
type RelayBuilder struct {
	ToolName string
	Config   RelayConfig
}

// Build captures the previous result for current-value batch selectors.
func (b RelayBuilder) Build(previous core.Result) core.Command {
	return &relayCommand{toolName: b.ToolName, config: b.Config, previous: previous}
}

type relayCommand struct {
	toolName string
	config   RelayConfig
	previous core.Result
	view     core.CommandStateView
}

func (c *relayCommand) Name() string { return c.toolName }

func (c *relayCommand) SetCommandState(view core.CommandStateView) { c.view = view }

func (c *relayCommand) Execute() core.Result {
	return c.ExecuteContext(context.Background())
}

func (c *relayCommand) ExecuteContext(ctx context.Context) core.Result {
	request, err := resolveBatch(c.config.BatchSource, c.previous.Output, c.view)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	timeout := c.config.Timeout
	if timeout == 0 {
		timeout = defaultRelayTimeout
	}
	exportCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := grpc.NewClient(
		c.config.Endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("connect relay endpoint %s: %w", c.config.Endpoint, err))
	}
	defer conn.Close()
	response, err := coltracepb.NewTraceServiceClient(conn).Export(exportCtx, request)
	if err != nil {
		if exportCtx.Err() != nil {
			err = exportCtx.Err()
		}
		return receiverError(c.Name(), fmt.Errorf("relay spans to %s: %w", c.config.Endpoint, err))
	}
	rejected := int(response.GetPartialSuccess().GetRejectedSpans())
	if rejected > 0 {
		return receiverError(c.Name(), fmt.Errorf(
			"relay spans to %s: %d spans rejected: %s",
			c.config.Endpoint, rejected, response.GetPartialSuccess().GetErrorMessage(),
		))
	}
	output, err := json.Marshal(map[string]any{
		"endpoint": c.config.Endpoint, "span_count": requestSpanCount(request),
		"accepted_span_count": requestSpanCount(request),
	})
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("SpansRelayed"), CommandName: c.Name(), Output: string(output)}
}

func (c *relayCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

var (
	_ core.ContextCommand    = (*relayCommand)(nil)
	_ core.CommandStateAware = (*relayCommand)(nil)
)

func validateRelayConfig(toolName string, config RelayConfig) error {
	if config.Endpoint == "" {
		return fmt.Errorf("tool %q config requires endpoint", toolName)
	}
	if _, _, err := net.SplitHostPort(config.Endpoint); err != nil {
		return fmt.Errorf("tool %q config has invalid endpoint %q", toolName, config.Endpoint)
	}
	if config.ReceiverAddress != "" {
		if _, _, err := net.SplitHostPort(config.ReceiverAddress); err != nil {
			return fmt.Errorf("tool %q config has invalid receiver_address %q", toolName, config.ReceiverAddress)
		}
		if normalizedEndpoint(config.Endpoint) == normalizedEndpoint(config.ReceiverAddress) {
			return fmt.Errorf(
				"tool %q config relays to its own receiver address %q",
				toolName, config.ReceiverAddress,
			)
		}
	}
	if config.Timeout <= 0 {
		return fmt.Errorf("tool %q config timeout must be positive", toolName)
	}
	if _, ok := core.ParseSelector(config.BatchSource); !ok {
		return fmt.Errorf("tool %q config has invalid batch_source %q", toolName, config.BatchSource)
	}
	return nil
}

func normalizedEndpoint(endpoint string) string {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return strings.ToLower(endpoint)
	}
	parsed := net.ParseIP(strings.Trim(host, "[]"))
	if host == "" || parsed == nil && strings.EqualFold(host, "localhost") ||
		parsed != nil && (parsed.IsLoopback() || parsed.IsUnspecified()) {
		host = "local"
	}
	return net.JoinHostPort(strings.ToLower(host), port)
}
