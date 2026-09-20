// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// The shared chat failure vocabulary a ToolDef opts into (srd058 R1.4, R2.4).
// The names are duplicated from the dialect package rather than imported,
// because catalog is below it: validation reads declarations, not clients.
var providerFailureSignals = []string{
	"ProviderUnauthorized", "ProviderThrottled", "ProviderUnavailable",
}

// ValidateProviderFailureOptIn checks that a word opting into the chat failure
// taxonomy also declares the signals it will emit (srd058 R2.5). The
// declaration stays the authority on what a machine must route, so a word that
// asks for the signals without declaring them would emit what no machine is
// required to handle.
func ValidateProviderFailureOptIn(defs []ToolDef) error {
	var failures []string
	for _, def := range defs {
		if def.Init != "invoke_llm" {
			continue
		}
		var cfg LLMToolConfig
		if err := DecodeToolConfig(def, &cfg); err != nil {
			continue // the word's own decode reports this with its context
		}
		if !cfg.ProviderFailureSignals {
			continue
		}
		declared := make(map[string]bool, len(def.Emits))
		for _, emit := range def.Emits {
			declared[emit] = true
		}
		var missing []string
		for _, signal := range providerFailureSignals {
			if !declared[signal] {
				missing = append(missing, signal)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			failures = append(failures, fmt.Sprintf(
				"tool %q sets provider_failure_signals but does not declare %s",
				def.Name, strings.Join(missing, ", ")))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("provider failure opt-in is incomplete: %s", strings.Join(failures, "; "))
	}
	return nil
}
