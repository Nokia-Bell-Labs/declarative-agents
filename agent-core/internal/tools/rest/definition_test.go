// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDefinitionLoadsValidHandAuthoredConfig(t *testing.T) {
	t.Parallel()

	def, err := ParseDefinition([]byte(validDefinitionYAML))
	require.NoError(t, err)
	require.Equal(t, "v1", def.Version)
	require.Equal(t, "https://api.github.com", def.Clients["github"].BaseURL)
	require.Contains(t, def.Clients["github"].Resources["issue"].Operations, "get")
	require.Equal(t, "127.0.0.1:0", def.Servers["control"].Address)
}

var validDefinitionYAML = string(mustReadFixture("valid_definition.yaml"))

func mustReadFixture(name string) []byte {
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		panic(err)
	}
	return data
}
