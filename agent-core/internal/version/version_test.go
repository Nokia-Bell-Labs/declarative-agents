// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringDevDefaults(t *testing.T) {
	t.Parallel()
	require.Equal(t, "v0.0.0-dev", Version)
	require.Equal(t, unknown, Commit)
	require.Equal(t, unknown, Date)
	require.Equal(t, "v0.0.0-dev", String())
}

func TestFormatInjectedForm(t *testing.T) {
	t.Parallel()
	require.Equal(t, "v1.2.3 (commit abcdef0, built 2026-08-20)",
		format("v1.2.3", "abcdef0", "2026-08-20"))
	require.Equal(t, "v1.2.3 (built 2026-08-20)",
		format("v1.2.3", unknown, "2026-08-20"))
	require.Equal(t, "v1.2.3 (commit abcdef0)",
		format("v1.2.3", "abcdef0", unknown))
	require.Equal(t, "v1.2.3", format("v1.2.3", unknown, unknown))
}
