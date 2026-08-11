// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestValidateClientEmitsRejectsSuccessAndFailureDrift(t *testing.T) {
	t.Parallel()
	operation := ClientOperationDefinition{
		OperationName: "get",
		Operation: Operation{
			Success:  StatusMapping{Signal: "Read"},
			Failures: []StatusMapping{{Signal: "Missing"}},
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
		Operation:     Operation{Success: StatusMapping{Signal: "RemoteFinished"}},
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
		Server: Server{LifecycleExit: LifecycleExitInjection{Disabled: true},
			Endpoints: map[string]Endpoint{
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
