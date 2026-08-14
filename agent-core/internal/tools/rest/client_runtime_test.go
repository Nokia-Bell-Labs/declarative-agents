// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTClient_SyncResourceWords(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(issueHandler))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())

	requireClientSignal(t, def, InitClientGet, "get", params("1"), "RESTResourceRead")
	requireClientSignal(t, def, InitClientSet, "set", params("1", "new"), "RESTResourceWritten")
	requireClientSignal(t, def, InitClientGet, "get", params("missing"), "RESTMissing")
	requireClientSignal(t, def, InitClientSet, "set", params("domain", "bad"), "RESTDomainFailed")
	requireClientSignal(t, def, InitClientGet, "get", params("boom"), string(core.CommandError))
}

func TestRESTClient_RenderCatchAllPathParam(t *testing.T) {
	t.Parallel()

	path := renderPath("/api/v1/docs/{path...}", map[string]interface{}{
		"path": "specs/use-cases/rel03.0-uc007-machine-request-documentation-ux.yaml",
	})

	require.Equal(t, "/api/v1/docs/specs/use-cases/rel03.0-uc007-machine-request-documentation-ux.yaml", path)
}

func TestRESTClient_MutatingOperationsRequireEffects(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateDefinition(mutatingDefinition(validWriteOperation())))

	missingEffects := validWriteOperation()
	missingEffects.SideEffects = nil
	require.ErrorContains(t, ValidateDefinition(mutatingDefinition(missingEffects)), "side_effects")

	irreversible := validWriteOperation()
	irreversible.Reversibility = Reversibility{Classification: "irreversible"}
	irreversible.Compensation = nil
	require.ErrorContains(t, ValidateDefinition(mutatingDefinition(irreversible)), "confirmation")

	irreversible.Reversibility.RequiresConfirmation = true
	require.NoError(t, ValidateDefinition(mutatingDefinition(irreversible)))

	missingCompensation := validWriteOperation()
	missingCompensation.Compensation = nil
	require.ErrorContains(t, ValidateDefinition(mutatingDefinition(missingCompensation)), "compensation target")
}

func TestRESTClientFactoryRollbackPolicyMatchesResolvedOperation(t *testing.T) {
	t.Parallel()

	write := validWriteOperation()
	read := validReadOperation()
	irreversible := validWriteOperation()
	irreversible.Reversibility = Reversibility{
		Classification: "irreversible", Undo: "irreversible", RequiresConfirmation: true,
	}
	irreversible.Compensation = nil
	definition := Definition{
		Version: "v1",
		Clients: map[string]Client{"github": {
			Operations: map[string]Operation{
				"read":         read,
				"write":        write,
				"irreversible": irreversible,
				"set":          write,
			},
		}},
	}
	require.NoError(t, ValidateDefinition(definition))
	collection := NewCollection()
	require.NoError(t, collection.Add(definition))

	tests := []struct {
		name           string
		operation      string
		classification string
		undo           string
		wantErr        string
	}{
		{
			name: "read alias remains reversible noop", operation: "read",
			classification: "reversible", undo: "noop",
		},
		{
			name: "compensatable alias uses receipt-producing operation", operation: "write",
			classification: "compensatable", undo: "compensating_action",
		},
		{
			name: "irreversible alias remains explicit", operation: "irreversible",
			classification: "irreversible", undo: "irreversible",
		},
		{
			name: "compensatable alias rejects read operation", operation: "read",
			classification: "compensatable", undo: "compensating_action",
			wantErr: "cannot produce a rollback receipt",
		},
		{
			name: "noop alias rejects receipt-producing operation", operation: "write",
			classification: "reversible", undo: "noop",
			wantErr: "disagrees with receipt-producing",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tool := catalog.ToolDef{
				Name:          "alias_" + tc.operation,
				Emits:         []string{"RESTResourceRead", "CommandError"},
				Reversibility: catalog.ToolReversibility{Classification: tc.classification},
				Undo:          catalog.ToolUndoContract{Strategy: tc.undo},
				Config: map[string]interface{}{
					"rest_ref": "github", "operation": tc.operation,
				},
			}

			_, err := newClientBuilder(tool, InitClientInvoke, FactoryDeps{Definitions: collection})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
