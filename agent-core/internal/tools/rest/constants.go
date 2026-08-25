// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"

const (
	authNone        = "none"
	authBasic       = "basic"
	authBearer      = "bearer"
	authHeaderToken = "header_token"
	authQueryToken  = "query_token"

	bodySourceParams         = "params"
	bodySourcePreviousResult = "previous_result"
	bodySourceNone           = "none"
	bodySourceCommandState   = "command_state"

	redirectNone      = "none"
	redirectSameHost  = "same_host"
	redirectAllowlist = "allowlist"

	bindingDynamicSignal = "emit_dynamic_signal"
	bindingStaticAssets  = "static_assets"
	bindingRedirect      = "redirect"

	queueOverflowReject     = "reject"
	queueOverflowDropOldest = "drop_oldest"
	queueOverflowDropNewest = "drop_newest"

	shutdownPolicyDrain         = "drain"
	shutdownPolicyDrainThenStop = "drain_then_stop"
)

var signalSourceAuthorityFields = map[string]bool{
	"signal": true, "profile": true, "profile_path": true, "machine": true, "method": true, "url": true, "host": true,
	"machine_spec": true, "tools": true, "tool_declarations": true, "model": true, "model_config": true, "checkpoint": true, "checkpoint_connection": true,
}

func shutdownDrainPolicy(shutdown restdef.ShutdownConfig) string {
	if shutdown.DrainPolicy != "" {
		return shutdown.DrainPolicy
	}
	return shutdownPolicyDrainThenStop
}
