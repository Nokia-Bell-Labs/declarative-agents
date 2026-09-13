// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

func TestCanonicalYAMLSortsMapKeys(t *testing.T) {
	data, err := canonicalYAML(struct {
		Config map[string]any `yaml:"config"`
	}{
		Config: map[string]any{
			"zulu":  map[string]any{"two": 2, "one": 1},
			"alpha": true,
		},
	})

	require.NoError(t, err)
	output := string(data)
	require.Less(t, strings.Index(output, "alpha:"), strings.Index(output, "zulu:"))
	require.Less(t, strings.Index(output, "one:"), strings.Index(output, "two:"))
}

func TestDumpConfigUsesCapturedBytesAndSortsTools(t *testing.T) {
	path := filepath.Join(t.TempDir(), "declarations.yaml")
	loaded := []byte("before\n")
	require.NoError(t, os.WriteFile(path, loaded, 0o600))
	closure := &Closure{
		Selected: []catalog.ToolDef{
			{Name: "zulu", Binary: "true", Config: map[string]any{"z": 1, "a": 2}},
			{Name: "alpha", Binary: "true"},
		},
		Rest:   toolrest.Collection{Version: "v1"},
		Files:  []string{path},
		Assets: map[string][]byte{path: loaded},
	}
	require.NoError(t, os.WriteFile(path, []byte("after\n"), 0o600))
	var output bytes.Buffer

	require.NoError(t, DumpConfig(closure, &output))

	text := output.String()
	require.Less(t, strings.Index(text, "name: alpha"), strings.Index(text, "name: zulu"))
	require.Contains(t, text, "version: v1")
	sum := sha256.Sum256(loaded)
	require.Contains(t, text, hex.EncodeToString(sum[:]))
	after := sha256.Sum256([]byte("after\n"))
	require.NotContains(t, text, hex.EncodeToString(after[:]))
}
