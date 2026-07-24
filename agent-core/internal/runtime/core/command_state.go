// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// CommandStateView is a read-only forward view over a run's execution log. Any
// command family can resolve a prior step's output by label without threading it
// through intervening steps. The view is receipt-blind: it exposes step outputs
// only and can never reach an Entry's opaque Receipt, so a forward selector
// cannot acquire transport authority or a receipt
// (srd038-command-state-store R1, R3).
type CommandStateView interface {
	// Lookup returns the output of the most recent prior step whose authored
	// label or command name matches the address. A miss returns ok=false and
	// never an error, so selector resolution can raise a typed error
	// (srd038-command-state-store R2, R1.5).
	Lookup(label string) (output string, ok bool)
}

// commandStateEntry is the receipt-blind projection of an Execution Entry that
// the view exposes. It deliberately omits Entry.Receipt so the receipt is
// unreachable by construction: a compile error, not a runtime filter, guards the
// boundary (srd038-command-state-store R3.3).
type commandStateEntry struct {
	label            string
	commandName      string
	output           string
	redactionVersion uint16
	redactionStatus  OutputRedactionStatus
}

type commandStateView struct {
	entries []commandStateEntry
}

// NewCommandStateView projects the execution log into the receipt-blind view.
// It retains both the optional authored label and the executed command name so
// either can address a step. The projection never reads the receipt.
func NewCommandStateView(execution Execution) CommandStateView {
	projected := make([]commandStateEntry, 0, len(execution))
	for _, e := range execution {
		output := e.Result.Output
		status := e.Result.RedactionStatus
		if e.Result.RedactionVersion == OutputRedactionVersion1 &&
			status == OutputRedactionApplied {
			output, _, status = applyOutputRedaction(
				output,
				e.Result.RedactionVersion,
				e.Result.RedactedPaths,
			)
		}
		projected = append(projected, commandStateEntry{
			label:            e.Label,
			commandName:      e.CommandName,
			output:           output,
			redactionVersion: e.Result.RedactionVersion,
			redactionStatus:  status,
		})
	}
	return &commandStateView{entries: projected}
}

// Lookup scans from the most recent entry backward and matches either authored
// label or command name. One scan makes duplicate labels and cross-address
// collisions resolve to the highest step index (srd038 R2.2, R2.7).
func (v *commandStateView) Lookup(label string) (string, bool) {
	output, found, err := v.lookup(label)
	return output, found && err == nil
}

// lookup preserves the public CommandStateView compatibility surface while
// giving selector resolution a typed reason when the newest matching entry is
// unsafe. An unavailable newest match does not fall back to an older match.
func (v *commandStateView) lookup(label string) (string, bool, error) {
	for i := len(v.entries) - 1; i >= 0; i-- {
		entry := v.entries[i]
		if (entry.label != "" && entry.label == label) || entry.commandName == label {
			if entry.redactionVersion != OutputRedactionVersion1 ||
				entry.redactionStatus != OutputRedactionApplied {
				return "", true, &CommandStateOutputUnavailableError{
					Label:   label,
					Version: entry.redactionVersion,
					Status:  entry.redactionStatus,
				}
			}
			return entry.output, true, nil
		}
	}
	return "", false, nil
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

// ParsedSelector is the canonical AST for $.path and $from(label).path selectors.
// An empty Label identifies the current value; Path always has nonempty components.
type ParsedSelector struct {
	Label string
	Path  []string
}

// ParseSelector parses the shared dotted-selector grammar used by validation and
// resolution. Empty components, whitespace/control characters, malformed
// parentheses, and noncanonical prefixes are rejected.
func ParseSelector(selector string) (ParsedSelector, bool) {
	var parsed ParsedSelector
	var path string
	switch {
	case strings.HasPrefix(selector, "$."):
		path = strings.TrimPrefix(selector, "$.")
	case strings.HasPrefix(selector, "$from("):
		remainder := strings.TrimPrefix(selector, "$from(")
		closeIdx := strings.Index(remainder, ")")
		if closeIdx <= 0 {
			return ParsedSelector{}, false
		}
		parsed.Label = remainder[:closeIdx]
		if !validSelectorLabel(parsed.Label) {
			return ParsedSelector{}, false
		}
		after := remainder[closeIdx+1:]
		if !strings.HasPrefix(after, ".") {
			return ParsedSelector{}, false
		}
		path = strings.TrimPrefix(after, ".")
	default:
		return ParsedSelector{}, false
	}
	parsed.Path = strings.Split(path, ".")
	for _, component := range parsed.Path {
		if !validSelectorComponent(component) {
			return ParsedSelector{}, false
		}
	}
	return parsed, true
}

func validSelectorLabel(label string) bool {
	return validSelectorComponent(label) &&
		!strings.ContainsAny(label, "().")
}

func validSelectorComponent(component string) bool {
	if component == "" || strings.TrimSpace(component) != component {
		return false
	}
	for _, r := range component {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Resolve walks a parsed selector path against one decoded JSON object.
func (s ParsedSelector) Resolve(source map[string]interface{}) (interface{}, bool) {
	var current interface{} = source
	for _, key := range s.Path {
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

// ParseFromSelector splits a $from(label).dotted.path selector into its label and
// path. It returns ok=false for current-value or malformed selectors.
func ParseFromSelector(selector string) (label, path string, ok bool) {
	parsed, ok := ParseSelector(selector)
	if !ok || parsed.Label == "" {
		return "", "", false
	}
	return parsed.Label, strings.Join(parsed.Path, "."), true
}

// UnresolvedLabelError reports a selector whose authored label or command name
// does not match any completed step.
type UnresolvedLabelError struct {
	Label string
}

func (e *UnresolvedLabelError) Error() string {
	return fmt.Sprintf("no prior step labeled %q", e.Label)
}

// UnresolvedPathError reports a selector path absent from the matched step's
// output. Label and Path retain the authored selector components for callers.
type UnresolvedPathError struct {
	Label string
	Path  string
}

func (e *UnresolvedPathError) Error() string {
	return fmt.Sprintf("path %q not found in step %q output", e.Path, e.Label)
}

// CommandStateOutputUnavailableError reports a matched Entry whose output
// cannot cross the selector boundary because its redaction metadata is missing,
// unknown, or records whole-output omission (srd038 R1.8, R5.6).
type CommandStateOutputUnavailableError struct {
	Label   string
	Version uint16
	Status  OutputRedactionStatus
}

func (e *CommandStateOutputUnavailableError) Error() string {
	return fmt.Sprintf(
		"output for step %q is unavailable to command-state selectors (redaction version %d, status %q)",
		e.Label,
		e.Version,
		e.Status,
	)
}

// ResolveFromSelector resolves a $from(label).path selector against the
// command-state view: it looks up the most recent step labeled label, decodes its
// output, and walks the dotted path. It returns a typed error for a malformed
// selector, an absent view, an unresolved label, non-JSON output, or a missing
// path, so a caller never silently reads an empty value (srd038 R1.5, R2, R3).
func ResolveFromSelector(view CommandStateView, selector string) (interface{}, error) {
	parsed, ok := ParseSelector(selector)
	if !ok || parsed.Label == "" {
		return nil, fmt.Errorf("selector %q is not a $from(label).path selector", selector)
	}
	label := parsed.Label
	path := strings.Join(parsed.Path, ".")
	if view == nil {
		return nil, fmt.Errorf("selector %q: no command-state view is configured", selector)
	}
	var output string
	var found bool
	if detailed, ok := view.(interface {
		lookup(string) (string, bool, error)
	}); ok {
		var err error
		output, found, err = detailed.lookup(label)
		if err != nil {
			return nil, fmt.Errorf("selector %q: %w", selector, err)
		}
	} else {
		output, found = view.Lookup(label)
	}
	if !found {
		return nil, fmt.Errorf("selector %q: %w", selector, &UnresolvedLabelError{Label: label})
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		return nil, fmt.Errorf("selector %q: step %q output is not a JSON object", selector, label)
	}
	value, ok := parsed.Resolve(decoded)
	if !ok {
		return nil, fmt.Errorf(
			"selector %q: %w",
			selector,
			&UnresolvedPathError{Label: label, Path: path},
		)
	}
	return value, nil
}
