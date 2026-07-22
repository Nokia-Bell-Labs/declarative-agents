// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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
	require.ErrorContains(t, ValidateDefinition(mutatingDefinition(irreversible)), "confirmation")

	compensating := validWriteOperation()
	compensating.Compensation = map[string]interface{}{"operation": "restore_issue"}
	require.NoError(t, ValidateDefinition(mutatingDefinition(compensating)))
}
