// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
)

func TestValidateClientEmitsRejectsSuccessAndFailureDrift(t *testing.T) {
	t.Parallel()
	operation := ClientOperationDefinition{
		OperationName: "get",
		Operation: restdef.Operation{
			Success:  restdef.StatusMapping{Signal: "Read"},
			Failures: []restdef.StatusMapping{{Signal: "Missing"}},
		},
	}
	def := catalog.ToolDef{Name: "get", Emits: []string{"Read", "CommandError"}}
	err := validateClientEmits(def, InitClientGet, operation)
	require.ErrorContains(t, err, "Missing")
	require.ErrorContains(t, err, "declared emits: Read, CommandError")

	def.Emits = append(def.Emits, "Missing")
	require.NoError(t, validateClientEmits(def, InitClientGet, operation))
}

func TestValidateClientSendUsesAcceptedSignal(t *testing.T) {
	t.Parallel()
	operation := ClientOperationDefinition{
		OperationName: "start",
		Operation:     restdef.Operation{Success: restdef.StatusMapping{Signal: "RemoteFinished"}},
	}
	def := catalog.ToolDef{
		Name: "send", Emits: []string{"RESTAccepted", "CommandError"},
	}
	require.NoError(t, validateClientEmits(def, InitClientSend, operation))
}

func TestValidateServerAwaitEmitsIncludesEndpointSignals(t *testing.T) {
	t.Parallel()
	server := ServerDefinition{
		Name: "control",
		Server: restdef.Server{LifecycleExit: restdef.LifecycleExitInjection{Disabled: true},
			Endpoints: map[string]restdef.Endpoint{
				"exit": {Signal: "ExitRequested"},
			}},
	}
	def := catalog.ToolDef{
		Name: "await", Emits: []string{"AwaitTimedOut", "ServerStopped", "CommandError"},
	}
	require.ErrorContains(t, validateServerAwaitEmits(def, server), "ExitRequested")
	def.Emits = append(def.Emits, "ExitRequested")
	require.NoError(t, validateServerAwaitEmits(def, server))
}
