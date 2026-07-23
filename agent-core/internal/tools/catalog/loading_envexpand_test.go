// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

// writeDecl writes a declarations file whose invoke_llm provider_url is the
// given literal, and returns its path.
func writeDecl(t *testing.T, providerURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "declarations.yaml")
	body := "tools:\n" +
		"  - name: answer\n" +
		"    type: builtin\n" +
		"    init: invoke_llm\n" +
		"    config:\n" +
		"      model: \"m\"\n" +
		"      provider: ollama\n" +
		"      provider_url: \"" + providerURL + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write declarations: %v", err)
	}
	return path
}

func providerURLOf(t *testing.T, defs []ToolDef) string {
	t.Helper()
	if len(defs) != 1 {
		t.Fatalf("loaded %d tool defs, want 1", len(defs))
	}
	value, ok := defs[0].Config["provider_url"].(string)
	if !ok {
		t.Fatalf("provider_url is %T, want string", defs[0].Config["provider_url"])
	}
	return value
}

// TestLoadToolDefsExpandsEnvReference is the GH-728 regression: a declaration
// that hard-codes localhost cannot be reached by a deployment, so the address
// has to be an environment reference the loader expands (srd013 R5.6).
func TestLoadToolDefsExpandsEnvReference(t *testing.T) {
	t.Setenv("OLLAMA_URL", "http://ollama.svc:11434")

	defs, err := LoadToolDefs(writeDecl(t, "${OLLAMA_URL:-http://localhost:11434}"))

	if err != nil {
		t.Fatalf("LoadToolDefs() error: %v", err)
	}
	if got := providerURLOf(t, defs); got != "http://ollama.svc:11434" {
		t.Fatalf("provider_url = %q, want the environment value", got)
	}
}

// TestLoadToolDefsUsesDefaultWhenUnset covers srd013 R5.7: the shipped
// declaration has to run unconfigured, which is what keeps the local targets
// working with no environment set.
func TestLoadToolDefsUsesDefaultWhenUnset(t *testing.T) {
	// Register restoration of the original value before exercising the unset case.
	t.Setenv("OLLAMA_URL", "")
	if err := os.Unsetenv("OLLAMA_URL"); err != nil {
		t.Fatalf("unset OLLAMA_URL: %v", err)
	}

	defs, err := LoadToolDefs(writeDecl(t, "${OLLAMA_URL:-http://localhost:11434}"))

	if err != nil {
		t.Fatalf("LoadToolDefs() error: %v", err)
	}
	if got := providerURLOf(t, defs); got != "http://localhost:11434" {
		t.Fatalf("provider_url = %q, want the declared default", got)
	}
}

// TestLoadToolDefsLeavesSelectorsUntouched guards the reason the pattern is
// brace-delimited: declarations carry $-prefixed selectors and $tool dispatch
// markers that must survive expansion byte-identical.
func TestLoadToolDefsLeavesSelectorsUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "declarations.yaml")
	body := "tools:\n" +
		"  - name: answer\n" +
		"    type: builtin\n" +
		"    init: invoke_llm\n" +
		"    config:\n" +
		"      model: \"m\"\n" +
		"      provider: ollama\n" +
		"      provider_url: \"http://localhost:11434\"\n" +
		"      user_prompt_from: \"$from(compose).prompt\"\n" +
		"      answer_path: \"$.output.answer\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write declarations: %v", err)
	}

	defs, err := LoadToolDefs(path)

	if err != nil {
		t.Fatalf("LoadToolDefs() error: %v", err)
	}
	for field, want := range map[string]string{
		"user_prompt_from": "$from(compose).prompt",
		"answer_path":      "$.output.answer",
	} {
		if got, _ := defs[0].Config[field].(string); got != want {
			t.Errorf("%s = %q, want %q unchanged by expansion", field, got, want)
		}
	}
}
