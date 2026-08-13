// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpoolGetMetricOutputDescribesPagination(t *testing.T) {
	t.Parallel()
	defs, err := LoadToolDeclarations([]string{builtinBundlePath(t)})
	require.NoError(t, err)
	description := toolDefByName(t, defs, "spool_get_metric").Output.Description
	require.Contains(t, strings.ToLower(description), "page")
	require.Contains(t, strings.ToLower(description), "total")
	require.NotContains(t, description, "All spooled records")
}
