// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"strings"
	"testing"
)

func optingInToolDef(emits ...string) ToolDef {
	return ToolDef{
		Name: "invoke_llm", Init: "invoke_llm", Emits: emits,
		Config: map[string]interface{}{
			"model": "m", "manifest_state": "Composing",
			"provider_failure_signals": true,
		},
	}
}

func TestProviderFailureOptInRequiresDeclaredEmits(t *testing.T) {
	err := ValidateProviderFailureOptIn([]ToolDef{
		optingInToolDef("LLMResponded", "CommandError"),
	})
	if err == nil {
		t.Fatal("an opt-in that declares none of the signals was accepted")
	}
	for _, want := range []string{
		"invoke_llm", "ProviderUnauthorized", "ProviderThrottled", "ProviderUnavailable",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestProviderFailureOptInNamesOnlyWhatIsMissing(t *testing.T) {
	err := ValidateProviderFailureOptIn([]ToolDef{
		optingInToolDef("LLMResponded", "CommandError", "ProviderUnavailable", "ProviderThrottled"),
	})
	if err == nil {
		t.Fatal("a partial declaration was accepted")
	}
	if strings.Contains(err.Error(), "ProviderUnavailable") {
		t.Errorf("error %q names a signal that was declared", err)
	}
	if !strings.Contains(err.Error(), "ProviderUnauthorized") {
		t.Errorf("error %q does not name the missing signal", err)
	}
}

func TestProviderFailureOptInAcceptsACompleteDeclaration(t *testing.T) {
	err := ValidateProviderFailureOptIn([]ToolDef{optingInToolDef(
		"LLMResponded", "CommandError",
		"ProviderUnauthorized", "ProviderThrottled", "ProviderUnavailable")})
	if err != nil {
		t.Fatalf("a complete declaration was rejected: %v", err)
	}
}

// A word that never opts in carries no obligation, which is what keeps every
// shipped profile running unmodified (srd058 R3.3).
func TestProviderFailureValidationIgnoresWordsThatDoNotOptIn(t *testing.T) {
	defs := []ToolDef{
		{Name: "invoke_llm", Init: "invoke_llm", Emits: []string{"LLMResponded", "CommandError"},
			Config: map[string]interface{}{"model": "m", "manifest_state": "Composing"}},
		{Name: "read", Init: "file_read", Emits: []string{"ToolDone", "ToolFailed"}},
	}
	if err := ValidateProviderFailureOptIn(defs); err != nil {
		t.Fatalf("a profile that does not opt in was rejected: %v", err)
	}
}
