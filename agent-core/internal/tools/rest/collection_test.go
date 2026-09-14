// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

func TestCollectionRetainsOneRESTVersion(t *testing.T) {
	collection := NewCollection()
	require.NoError(t, collection.Add(restdef.Definition{Version: "v1"}))
	require.Equal(t, "v1", collection.Version)
	require.NoError(t, collection.Add(restdef.Definition{Version: "v1"}))
	require.ErrorContains(t,
		collection.Add(restdef.Definition{Version: "v2"}),
		`conflicting REST versions "v1" and "v2"`,
	)
}
