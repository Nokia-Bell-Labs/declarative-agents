// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package typesys_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

// Pinning test for application-dsl-spec 1.0 R-TOOL-006: every declared type
// is named.
func TestSpecPinDeclaredTypeIsNamed(t *testing.T) {
	t.Parallel()
	_, err := typesys.Build(typesys.TypeUnitFile{
		Path: "/units/chat-types.yaml", Unit: "chat-types",
		Types: []typesys.TypeDecl{{Schema: map[string]any{"type": "string"}}},
	})
	require.ErrorContains(t, err, "declares a type with no name")
}
