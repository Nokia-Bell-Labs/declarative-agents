// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"strings"
)

// Signature consistency against the legacy contract blocks (srd051 R6.2 to
// R6.4). Each conflict is a load error naming the tool and both fields,
// because a tool that states the same thing twice has two places to drift.

// validateToolSignature rejects a signature that contradicts the legacy blocks
// beside it.
func validateToolSignature(def ToolDef) error {
	if def.Signature == nil {
		return nil
	}
	if err := validateSignatureEmits(def); err != nil {
		return err
	}
	if def.Signature.Output != "" && len(def.Output.Schema) > 0 {
		return fmt.Errorf(
			"tool %q declares both signature.output and output.schema", def.Name)
	}
	if def.Signature.Input != "" && len(def.Parameters) > 0 {
		return fmt.Errorf(
			"tool %q declares both signature.input and parameters", def.Name)
	}
	return nil
}

// validateSignatureEmits accepts a legacy list identical to the signature's, so
// a migration converts one file at a time rather than the whole corpus at once
// (srd051 R6.2).
func validateSignatureEmits(def ToolDef) error {
	if len(def.Signature.Emits) == 0 || len(def.Emits) == 0 {
		return nil
	}
	if equalSignalLists(def.Signature.Emits, def.Emits) {
		return nil
	}
	return fmt.Errorf(
		"tool %q declares signature.emits [%s] and emits [%s], which differ",
		def.Name, strings.Join(def.Signature.Emits, " "), strings.Join(def.Emits, " "))
}

func equalSignalLists(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// applySignatureEmits folds a signature's emitted signals into the legacy field
// so every existing reader of Emits sees them without knowing about
// signatures. Nothing downstream distinguishes the two forms.
func applySignatureEmits(def ToolDef) ToolDef {
	if def.Signature == nil || len(def.Signature.Emits) == 0 || len(def.Emits) > 0 {
		return def
	}
	def.Emits = append([]string(nil), def.Signature.Emits...)
	return def
}
