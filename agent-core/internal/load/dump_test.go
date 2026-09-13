// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
