// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CommandStateView is a read-only forward view over a run's execution log. Any
// command family can resolve a prior step's output by label without threading it
// through intervening steps. The view is receipt-blind: it exposes step outputs
// only and can never reach an Entry's opaque Receipt, so a forward selector
// cannot acquire transport authority or a receipt
// (srd038-command-state-store R1, R3).
type CommandStateView interface {
	// Lookup returns the output of the most recent prior step whose label
	// matches, resolving duplicates most-recent-wins. A miss returns ok=false
	// and never an error, so callers raise their own typed errors
	// (srd038-command-state-store R2, R1.5).
	Lookup(label string) (output string, ok bool)
}

// commandStateEntry is the receipt-blind projection of an Execution Entry that
// the view exposes. It deliberately omits Entry.Receipt so the receipt is
// unreachable by construction: a compile error, not a runtime filter, guards the
// boundary (srd038-command-state-store R3.3).
type commandStateEntry struct {
	label  string
	output string
}

type commandStateView struct {
	entries []commandStateEntry
}

// NewCommandStateView projects the execution log into the receipt-blind view.
// Labels resolve to a step's command name today; a future declared-label field
// can extend the projection without changing the interface. The projection reads
// only CommandName and Output from each Entry, never the receipt.
func NewCommandStateView(execution Execution) CommandStateView {
	projected := make([]commandStateEntry, 0, len(execution))
	for _, e := range execution {
		projected = append(projected, commandStateEntry{
			label:  e.CommandName,
			output: e.Result.Output,
		})
	}
	return &commandStateView{entries: projected}
}

// Lookup scans from the most recent entry backward so duplicate labels resolve
// to the highest step index (srd038-command-state-store R2.2).
func (v *commandStateView) Lookup(label string) (string, bool) {
	for i := len(v.entries) - 1; i >= 0; i-- {
		if v.entries[i].label == label {
			return v.entries[i].output, true
		}
	}
	return "", false
}

var _ CommandStateView = (*commandStateView)(nil)

// injectCommandState gives a CommandStateAware command a forward view built from
// the prior steps' entries. At dispatch time the execution log holds every step
// before the current one (the current entry is appended after dispatch), so the
// view is a strictly-forward, receipt-blind read over completed steps.
func injectCommandState(cmd Command, priorSteps Execution) {
	if aware, ok := cmd.(CommandStateAware); ok {
		aware.SetCommandState(NewCommandStateView(priorSteps))
	}
}

// ParseFromSelector splits a $from(label).dotted.path selector into its label and
// path. It returns ok=false for any other selector form, so callers can reject a
// malformed or wrong-form selector (srd038-command-state-store R2).
func ParseFromSelector(selector string) (label, path string, ok bool) {
	const prefix = "$from("
	if !strings.HasPrefix(selector, prefix) {
		return "", "", false
	}
	remainder := selector[len(prefix):]
	closeIdx := strings.Index(remainder, ")")
	if closeIdx <= 0 {
		return "", "", false
	}
	label = remainder[:closeIdx]
	after := remainder[closeIdx+1:]
	if !strings.HasPrefix(after, ".") {
		return "", "", false
	}
	path = strings.TrimPrefix(after, ".")
	if label == "" || path == "" {
		return "", "", false
	}
	return label, path, true
}

// ResolveFromSelector resolves a $from(label).path selector against the
// command-state view: it looks up the most recent step labeled label, decodes its
// output, and walks the dotted path. It returns a typed error for a malformed
// selector, an absent view, an unresolved label, non-JSON output, or a missing
// path, so a caller never silently reads an empty value (srd038 R1.5, R2, R3).
func ResolveFromSelector(view CommandStateView, selector string) (interface{}, error) {
	label, path, ok := ParseFromSelector(selector)
	if !ok {
		return nil, fmt.Errorf("selector %q is not a $from(label).path selector", selector)
	}
	if view == nil {
		return nil, fmt.Errorf("selector %q: no command-state view is configured", selector)
	}
	output, found := view.Lookup(label)
	if !found {
		return nil, fmt.Errorf("selector %q: no prior step labeled %q", selector, label)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		return nil, fmt.Errorf("selector %q: step %q output is not a JSON object", selector, label)
	}
	value, ok := walkDottedPath(decoded, path)
	if !ok {
		return nil, fmt.Errorf("selector %q: path %q not found in step %q output", selector, path, label)
	}
	return value, nil
}

// walkDottedPath walks a dotted path (for example "mapped.embedding") into nested
// maps.
func walkDottedPath(source map[string]interface{}, path string) (interface{}, bool) {
	var current interface{} = source
	for _, key := range strings.Split(path, ".") {
		container, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = container[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
