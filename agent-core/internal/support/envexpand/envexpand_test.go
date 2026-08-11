// Copyright (c) 2026 Nokia. All rights reserved.

package envexpand

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandReplacesEnvironmentReference(t *testing.T) {
	t.Setenv("ENVEXPAND_VALUE", "configured")

	require.Equal(t, []byte("value=configured"), Expand([]byte("value=${ENVEXPAND_VALUE}")))
}

func TestExpandUsesDefaultForUndefinedVariable(t *testing.T) {
	unsetForTest(t, "ENVEXPAND_DEFAULT")

	require.Equal(t, []byte("value=fallback"), Expand([]byte("value=${ENVEXPAND_DEFAULT:-fallback}")))
}

func TestExpandRemovesUndefinedVariableWithoutDefault(t *testing.T) {
	unsetForTest(t, "ENVEXPAND_UNDEFINED")

	require.Equal(t, []byte("value="), Expand([]byte("value=${ENVEXPAND_UNDEFINED}")))
}

func TestExpandPreservesSelectorsExactly(t *testing.T) {
	input := []byte("$.path\n$from(label).field")

	require.Equal(t, input, Expand(input))
}

func unsetForTest(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "temporary")
	require.NoError(t, os.Unsetenv(name))
}
