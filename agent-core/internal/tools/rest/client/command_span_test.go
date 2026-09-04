// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
)

// The dispatch span's peer attribute (GH-1937): a REST invoke's span carries
// server.address from the declared client base URL, the way an LLM invoke's
// span carries its provider peer, so trace-derived topology sees every
// egress. A runtime-selected base URL resolves after span creation and
// leaves the span unstamped.

func spanAttrValue(cmd *clientCmd, key string) (string, bool) {
	for _, attr := range cmd.SpanCreationAttrs() {
		if string(attr.Key) == key {
			return attr.Value.AsString(), true
		}
	}
	return "", false
}

func TestClientSpanCarriesDeclaredPeerHost(t *testing.T) {
	t.Parallel()
	cmd := &clientCmd{toolName: "invoke_provider"}
	cmd.operation.Client.BaseURL = "https://api.cohere.com"
	host, ok := spanAttrValue(cmd, string(genai.AttrServerAddress))
	if !ok || host != "api.cohere.com" {
		t.Errorf("span server.address = %q (present=%v), want the declared base URL's host", host, ok)
	}
	if cmd.SpanName() != "execute_tool invoke_provider" {
		t.Errorf("SpanName() = %q; the override must keep the execute_tool identity", cmd.SpanName())
	}
}

func TestClientSpanKeepsPortAndSkipsUndeclaredBaseURL(t *testing.T) {
	t.Parallel()
	withPort := &clientCmd{toolName: "invoke_local"}
	withPort.operation.Client.BaseURL = "http://127.0.0.1:18085"
	host, _ := spanAttrValue(withPort, string(genai.AttrServerAddress))
	if host != "127.0.0.1:18085" {
		t.Errorf("span server.address = %q, want host:port preserved", host)
	}

	selected := &clientCmd{toolName: "invoke_selected"}
	if _, ok := spanAttrValue(selected, string(genai.AttrServerAddress)); ok {
		t.Error("an operation with no declared base URL must leave the span unstamped")
	}
}
