// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func objectSchema(required []string, properties map[string]string) map[string]interface{} {
	props := map[string]interface{}{}
	for name, kind := range properties {
		props[name] = map[string]interface{}{"type": kind}
	}
	req := make([]interface{}, 0, len(required))
	for _, name := range required {
		req = append(req, name)
	}
	return map[string]interface{}{"type": "object", "required": req, "properties": props}
}

func resolveThreadingOp(t *testing.T, def Definition, restRef, operation string) ClientOperationDefinition {
	t.Helper()
	collection := NewCollection()
	require.NoError(t, collection.Add(def))
	resolved, err := collection.ResolveClientOperation(ClientToolConfig{RestRef: restRef, Operation: operation})
	require.NoError(t, err)
	return resolved
}

func threadingCommand(op ClientOperationDefinition, prior core.Result) core.Command {
	return ClientBuilder{ToolName: op.OperationName, Init: InitClientInvoke, Operation: op}.Build(prior)
}
