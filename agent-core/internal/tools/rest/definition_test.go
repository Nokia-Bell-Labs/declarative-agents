// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func TestParseDefinitionRejectsUnknownFieldsAtEveryStructLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		old     string
		unknown string
	}{
		{name: "definition", old: "  version: v1", unknown: "  unknown_definition: true"},
		{name: "auth profile", old: "      token_ref: github_token", unknown: "      token_reff: github_token"},
		{name: "limit profile", old: "      timeout: 30s", unknown: "      max_response_byte: 1"},
		{name: "redirect policy", old: "        mode: same_host", unknown: "        max_redirect: 1"},
		{name: "client", old: "      auth_ref: github_app", unknown: "      auth_reff: github_app"},
		{name: "resource", old: "          path: /repos/{owner}/{repo}/issues/{number}", unknown: "          id_feld: id"},
		{name: "operation", old: "              method: GET", unknown: "              methd: GET"},
		{name: "request binding", old: "              params:", unknown: "                body_shema: {}"},
		{name: "status mapping", old: "              success: {status: [200], signal: RESTResourceRead}", unknown: "              success: {status: [200], signl: RESTResourceRead}"},
		{name: "server", old: "      address: 127.0.0.1:0", unknown: "      adress: 127.0.0.1:0"},
		{name: "endpoint", old: "          binding: emit_signal", unknown: "          bindng: emit_signal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := strings.Replace(validDefinitionYAML, tt.old, tt.old+"\n"+tt.unknown, 1)
			_, err := ParseDefinition([]byte(input))
			require.Error(t, err)
			assert.ErrorContains(t, err, strings.TrimSpace(strings.Split(tt.unknown, ":")[0]))
			assert.ErrorContains(t, err, "line ")
		})
	}
}

func TestDefinitionCanonicalYAMLRoundTrip(t *testing.T) {
	t.Parallel()
	def, err := ParseDefinition([]byte(validDefinitionYAML))
	require.NoError(t, err)
	encoded, err := yaml.Marshal(DefinitionFile{Rest: def})
	require.NoError(t, err)
	roundTrip, err := ParseDefinition(encoded)
	require.NoError(t, err)
	assert.Equal(t, def, roundTrip)
}

func FuzzParseDefinitionRejectsUnknownFields(f *testing.F) {
	f.Add([]byte("typo"))
	f.Add([]byte{0, 1, 2, 0xff})
	f.Fuzz(func(t *testing.T, suffix []byte) {
		key := fmt.Sprintf("unknown_%x", suffix)
		input := strings.Replace(validDefinitionYAML, "  version: v1", "  version: v1\n  "+key+": true", 1)
		if _, err := ParseDefinition([]byte(input)); err == nil {
			t.Fatalf("accepted unknown REST definition field %q", key)
		}
	})
}

var validDefinitionYAML = string(mustReadFixture("valid_definition.yaml"))

func mustReadFixture(name string) []byte {
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		panic(err)
	}
	return data
}
