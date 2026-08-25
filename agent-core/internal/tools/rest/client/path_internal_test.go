// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRESTClient_RenderCatchAllPathParam(t *testing.T) {
	t.Parallel()

	path := renderPath("/api/v1/docs/{path...}", map[string]interface{}{
		"path": "specs/use-cases/rel03.0-uc007-machine-request-documentation-ux.yaml",
	})

	require.Equal(t, "/api/v1/docs/specs/use-cases/rel03.0-uc007-machine-request-documentation-ux.yaml", path)
}
