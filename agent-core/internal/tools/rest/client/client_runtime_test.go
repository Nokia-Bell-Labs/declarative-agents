// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client_test

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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
