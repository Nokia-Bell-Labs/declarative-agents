// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import "fmt"

// Category-derived contract defaults for tools that declare a signature
// (srd051 R6.6 through R6.10).
//
// Defaults are applied once, at load, so the ToolDef every consumer sees
// already carries its effective contract. ValidateToolContracts and the
// corpus audit in pkg/spec therefore need no knowledge of this table and
// cannot disagree about it: they read the same populated fields they always
// read. The dump renders the effective contract for the same reason (R6.11).

// defaultingCategories lists the categories whose side-effect, reversibility,
// and undo obligations a signature discharges. Boundary is absent by design:
// a tool that writes to the world states what it writes, what that costs to
// reverse, and how, signature or not (srd051 R6.9).
var defaultingCategories = map[string]contractDefaults{
	"word":     {sideEffects: true, reversibility: true, undo: true},
	"response": {sideEffects: true, reversibility: true, undo: true},
	// stateful_internal keeps its side-effect declaration: it mutates state
	// the run carries forward, and which state is not derivable from a
	// category (srd051 R6.8).
	"stateful_internal": {reversibility: true, undo: true},
}

type contractDefaults struct {
	sideEffects   bool
	reversibility bool
	undo          bool
}

// applyContractDefaults fills the contract blocks a signature discharges,
// leaving every block the author stated untouched (srd051 R6.10).
func applyContractDefaults(def ToolDef) ToolDef {
	if def.Signature == nil {
		return def
	}
	// Folding the signature's signals into the legacy field lets every existing
	// reader of Emits see them without knowing about signatures.
	if len(def.Signature.Emits) > 0 && len(def.Emits) == 0 {
		def.Emits = append([]string(nil), def.Signature.Emits...)
	}
	def = applyProseDefaults(def)
	allowed, ok := defaultingCategories[contractCategory(def)]
	if !ok {
		return def
	}
	if allowed.sideEffects && def.SideEffects.LegacyText == "" && len(def.SideEffects.Items) == 0 {
		def.SideEffects.Items = []ToolSideEffect{{
			Kind:        "none",
			State:       "read_only",
			Description: fmt.Sprintf("%s computes a value and mutates nothing", def.Name),
		}}
	}
	if allowed.reversibility && def.Reversibility.Classification == "" {
		def.Reversibility.Classification = "reversible"
	}
	if allowed.undo && def.Undo.Strategy == "" && len(def.Requirements.Undo) == 0 {
		def.Undo.Strategy = "noop"
		def.Undo.Description = fmt.Sprintf("%s mutates nothing, so undo does nothing", def.Name)
	}
	return def
}

// applyProseDefaults derives the descriptive blocks from the description,
// whatever the category (srd051 R6.6). A signature states what a tool
// consumes, returns, and emits; repeating that in prose is what this epic set
// out to stop requiring.
func applyProseDefaults(def ToolDef) ToolDef {
	if def.Description == "" {
		return def
	}
	if def.Problem == "" {
		def.Problem = def.Description
	}
	if len(def.Goals) == 0 {
		def.Goals = []string{def.Description}
	}
	if len(def.NonGoals) == 0 {
		def.NonGoals = []string{
			fmt.Sprintf("Behavior outside %s, which its signature states in full", def.Name),
		}
	}
	def.Requirements = defaultedRequirements(def)
	return def
}

func defaultedRequirements(def ToolDef) ToolRequirements {
	requirements := def.Requirements
	if len(requirements.Input) == 0 {
		requirements.Input = []string{signatureRequirement(def.Signature.Input, "input", def)}
	}
	if len(requirements.Output) == 0 {
		requirements.Output = []string{signatureRequirement(def.Signature.Output, "output", def)}
	}
	if len(requirements.Errors) == 0 {
		requirements.Errors = []string{
			fmt.Sprintf("%s emits CommandError when it cannot produce its declared output", def.Name),
		}
	}
	return requirements
}

func signatureRequirement(ref, side string, def ToolDef) string {
	if ref == "" {
		return fmt.Sprintf("%s declares no %s type", def.Name, side)
	}
	return fmt.Sprintf("%s takes its %s as %s", def.Name, side, ref)
}

// SignatureDischarges reports which contract blocks a signature discharges for
// a category. It is the single statement of srd051 R6.7 through R6.9: the
// runtime contract check reads it through the defaults applied at load, and
// the corpus audit in pkg/spec reads it directly, so the two cannot disagree
// about what a signature covers.
//
// Boundary returns false for all three. A tool that writes to the world states
// what it writes, what that costs to reverse, and how.
func SignatureDischarges(category string) (sideEffects, reversibility, undo bool) {
	allowed, ok := defaultingCategories[category]
	if !ok {
		return false, false, false
	}
	return allowed.sideEffects, allowed.reversibility, allowed.undo
}

// SignatureDischargesProse reports whether a signature discharges the
// descriptive blocks. It does so for every category (srd051 R6.6).
func SignatureDischargesProse() bool { return true }
