// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "testing"

func TestExpandEnvSubstitutesAndDefaults(t *testing.T) {
	t.Setenv("RAG_COLLECTION", "corpus7")
	// CHROMA_URL is deliberately left unset so the default branch is exercised.

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"set var wins over default", "name: ${RAG_COLLECTION:-corpus}", "name: corpus7"},
		{"unset var uses default", "url: ${CHROMA_URL:-http://127.0.0.1:8000}", "url: http://127.0.0.1:8000"},
		{"bare set var", "name: ${RAG_COLLECTION}", "name: corpus7"},
		{"bare unset var expands empty", "name: ${MISSING_VAR}", "name: "},
		{"unset var empty default", "name: ${MISSING_VAR:-}", "name: "},
		{"default with colon preserved", "m: ${X:-qwen3-embedding:8b}", "m: qwen3-embedding:8b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(expandEnv([]byte(c.in))); got != c.want {
				t.Errorf("expandEnv(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestExpandEnvLeavesSelectorsUntouched(t *testing.T) {
	// JSONPath and $from selectors carry no brace, so they must survive verbatim,
	// as must Go-style {{ template }} bodies.
	inputs := []string{
		"embedding: $.embedding",
		"query_embeddings: $from(embed_query).mapped.embedding",
		`prompt: "{{ params.input }}"`,
		"documents: $.documents",
	}
	for _, in := range inputs {
		if got := string(expandEnv([]byte(in))); got != in {
			t.Errorf("expandEnv(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestParseDefinitionExpandsEnv(t *testing.T) {
	t.Setenv("RAG_BIND_HOST", "0.0.0.0")
	def, err := ParseDefinition([]byte(`
rest:
  version: v1
  auth:
    none: {type: none}
  limits:
    pub:
      network:
        allow_public_listener: true
  servers:
    api:
      address: ${RAG_BIND_HOST:-127.0.0.1}:18085
      limits_ref: pub
      endpoints:
        health: {method: GET, path: /health, binding: health}
`))
	if err != nil {
		t.Fatalf("ParseDefinition: %v", err)
	}
	if got := def.Servers["api"].Address; got != "0.0.0.0:18085" {
		t.Fatalf("server address = %q, want 0.0.0.0:18085", got)
	}
}
