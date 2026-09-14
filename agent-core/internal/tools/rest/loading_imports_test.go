// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefinitionsResolvesImportedRESTUnits(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared.yaml")
	top := filepath.Join(root, "top.yaml")
	require.NoError(t, os.WriteFile(shared, []byte(`unit: shared
rest:
  version: v1
  auth:
    bearer:
      type: bearer
      token_ref: API_TOKEN
`), 0o600))
	require.NoError(t, os.WriteFile(top, []byte(`unit: top
imports: [shared.yaml]
rest: {}
`), 0o600))

	collection, err := LoadDefinitions([]string{top}, nil)

	require.NoError(t, err)
	require.Equal(t, "v1", collection.Version)
	require.Equal(t, "API_TOKEN", collection.Auth["bearer"].TokenRef)
}
