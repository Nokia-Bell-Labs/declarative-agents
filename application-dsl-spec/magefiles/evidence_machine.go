// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Document checks for the machine-format statements (R-MACH-003 onward) and
// the stage-fragment and machine-template statements (R-MODEL-004 onward).
// Each is a pure YAML-shape check: it reads the fixture as plain data and
// never imports the engine whose load rules it mirrors.

// machTopKeynames is the closed set of keynames a machine-profile document
// may carry. `imports` is absent: the loader decodes it only to reject it.
var machTopKeynames = map[string]bool{
	"unit": true, "expand": true, "name": true, "purpose": true,
	"invariants": true, "lifecycle": true, "configuration": true,
	"metric_labels": true, "pipeline_diagram": true, "view_tags": true,
	"initial_state": true, "summary_signal": true, "resume_signal": true,
	"states": true, "terminal_states": true, "signals": true,
	"transitions": true, "budget": true, "external_labels": true,
}

// machTransitionKeynames is the closed set of keynames a transition entry
// may carry.
var machTransitionKeynames = map[string]bool{
	"state": true, "signal": true, "next": true, "action": true, "label": true,
	"report_output": true, "summary": true, "for_each": true, "metric_labels": true,
}

var machStateKeynames = map[string]bool{"name": true, "meaning": true, "run_status": true, "tags": true}

var machSignalKeynames = map[string]bool{"name": true, "trigger": true}

var machBudgetKeynames = map[string]bool{
	"max_iterations": true, "max_tokens": true, "max_duration": true,
	"command_timeout": true, "max_consecutive_parse_errors": true,
}

var machRunStatuses = map[string]bool{
	"succeeded": true, "failed": true, "budget_exceeded": true, "cancelled": true, "suspended": true,
}

// machForEachKeynames and machJoinKeynames close the for_each grammar.
var machForEachKeynames = map[string]bool{
	"items": true, "as": true, "mode": true, "max_concurrency": true, "failure": true,
	"continue_on": true, "abort_on": true, "join": true,
}

var machJoinKeynames = map[string]bool{"next": true, "signals": true, "label": true}

var machJoinSignalKeynames = map[string]bool{"all_success": true, "partial": true, "failed": true, "empty": true}

// checkMachineTopKeynames enforces R-MACH-003.
func checkMachineTopKeynames(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	return machClosedKeys("machine-profile document", doc, machTopKeynames)
}

// checkMachineTransitionKeynames enforces R-MACH-004.
func checkMachineTransitionKeynames(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	transitions, err := machTransitions(doc)
	if err != nil {
		return err
	}
	for index, transition := range transitions {
		label := fmt.Sprintf("transitions[%d]", index)
		if err := machClosedKeys(label, transition, machTransitionKeynames); err != nil {
			return err
		}
		for _, keyname := range []string{"state", "signal", "next"} {
			if machString(transition[keyname]) == "" {
				return fmt.Errorf("%s keyname %q is absent or not a non-empty string", label, keyname)
			}
		}
	}
	return nil
}

// checkMachineNamedSpecs enforces R-MACH-005: each states and signals entry
// is a name or a mapping carrying a name and only its form's keynames, and no
// name repeats within its list.
func checkMachineNamedSpecs(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	for _, list := range []struct {
		keyname string
		allowed map[string]bool
	}{{"states", machStateKeynames}, {"signals", machSignalKeynames}} {
		entries, ok := doc[list.keyname].([]any)
		if !ok {
			return fmt.Errorf("keyname %q is absent or not a sequence", list.keyname)
		}
		seen := map[string]bool{}
		for index, raw := range entries {
			label := fmt.Sprintf("%s[%d]", list.keyname, index)
			var name string
			switch entry := raw.(type) {
			case string:
				name = entry
			case map[string]any:
				if err := machClosedKeys(label, entry, list.allowed); err != nil {
					return err
				}
				name = machString(entry["name"])
			default:
				return fmt.Errorf("%s must be a name or a mapping", label)
			}
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("%s carries no name", label)
			}
			if seen[name] {
				return fmt.Errorf("%s repeats the name %q", label, name)
			}
			seen[name] = true
		}
	}
	return nil
}

// checkMachineGraphReferences enforces R-MACH-006.
func checkMachineGraphReferences(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	states := machNames(doc["states"])
	signals := machNames(doc["signals"])
	if len(states) == 0 {
		return errors.New("no state is declared")
	}
	initial := machString(doc["initial_state"])
	if initial == "" || !states[initial] {
		return fmt.Errorf("initial_state %q is absent or not a declared state", initial)
	}
	terminals, _ := doc["terminal_states"].([]any)
	if len(terminals) == 0 {
		return errors.New("no terminal state is declared")
	}
	seenTerminal := map[string]bool{}
	for index, raw := range terminals {
		name := machString(raw)
		if !states[name] {
			return fmt.Errorf("terminal_states[%d] %q is not a declared state", index, name)
		}
		if seenTerminal[name] {
			return fmt.Errorf("terminal_states[%d] repeats %q", index, name)
		}
		seenTerminal[name] = true
	}
	transitions, err := machTransitions(doc)
	if err != nil {
		return err
	}
	if len(transitions) == 0 {
		return errors.New("no transition is declared")
	}
	pairs := map[string]bool{}
	for index, transition := range transitions {
		state, signal, next := machString(transition["state"]), machString(transition["signal"]), machString(transition["next"])
		if !states[state] || !states[next] {
			return fmt.Errorf("transitions[%d] names an undeclared state", index)
		}
		if !signals[signal] {
			return fmt.Errorf("transitions[%d] signal %q is not declared", index, signal)
		}
		pair := state + "\x00" + signal
		if pairs[pair] {
			return fmt.Errorf("transitions[%d] repeats the state %q and signal %q", index, state, signal)
		}
		pairs[pair] = true
	}
	return nil
}

// checkMachineRunStatus enforces R-MACH-007: run_status takes a known value,
// stands only on a terminal state, and every terminal state declares one.
func checkMachineRunStatus(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	terminals := map[string]bool{}
	if list, ok := doc["terminal_states"].([]any); ok {
		for _, raw := range list {
			terminals[machString(raw)] = true
		}
	}
	declared := map[string]bool{}
	states, _ := doc["states"].([]any)
	for index, raw := range states {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		status, present := entry["run_status"]
		if !present {
			continue
		}
		name := machString(entry["name"])
		if !machRunStatuses[machString(status)] {
			return fmt.Errorf("states[%d].run_status %v is not a known run status", index, status)
		}
		if !terminals[name] {
			return fmt.Errorf("states[%d].run_status stands on %q, which is not terminal", index, name)
		}
		declared[name] = true
	}
	for name := range terminals {
		if !declared[name] {
			return fmt.Errorf("terminal state %q declares no run_status", name)
		}
	}
	return nil
}

// checkMachineBudget enforces R-MACH-008.
func checkMachineBudget(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	budget, ok := doc["budget"].(map[string]any)
	if !ok {
		return errors.New("budget is absent or not a mapping")
	}
	if err := machClosedKeys("budget", budget, machBudgetKeynames); err != nil {
		return err
	}
	if iterations, ok := budget["max_iterations"].(int); !ok || iterations <= 0 {
		return errors.New("budget.max_iterations is absent or not a positive integer")
	}
	timeout, err := time.ParseDuration(machString(budget["command_timeout"]))
	if err != nil || timeout <= 0 {
		return errors.New("budget.command_timeout is absent or not a positive duration")
	}
	for _, keyname := range []string{"max_tokens", "max_consecutive_parse_errors"} {
		if raw, present := budget[keyname]; present {
			if value, ok := raw.(int); !ok || value < 0 {
				return fmt.Errorf("budget.%s is not a non-negative integer", keyname)
			}
		}
	}
	return nil
}

// checkMachinePolicySignals enforces R-MACH-009.
func checkMachinePolicySignals(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	signals := machNames(doc["signals"])
	for _, keyname := range []string{"summary_signal", "resume_signal"} {
		if raw, present := doc[keyname]; present && !signals[machString(raw)] {
			return fmt.Errorf("%s %v is not a declared signal", keyname, raw)
		}
	}
	if _, present := doc["resume_signal"]; signals["AwaitApproval"] && !present {
		return errors.New("signals declares AwaitApproval but resume_signal is absent")
	}
	return nil
}

// checkMachineActionForms enforces R-MACH-011: report_output is a $.path
// selector, and report_output and a true summary each require an action.
func checkMachineActionForms(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	transitions, err := machTransitions(doc)
	if err != nil {
		return err
	}
	for index, transition := range transitions {
		label := fmt.Sprintf("transitions[%d]", index)
		action, present := transition["action"]
		if present && machString(action) == "" {
			return fmt.Errorf("%s.action is present but not a non-empty string", label)
		}
		if raw, present := transition["report_output"]; present {
			if !machDotPath(machString(raw)) {
				return fmt.Errorf("%s.report_output %v is not a $.path selector", label, raw)
			}
			if machString(action) == "" {
				return fmt.Errorf("%s.report_output requires an action", label)
			}
		}
		if summary, _ := transition["summary"].(bool); summary && machString(action) == "" {
			return fmt.Errorf("%s.summary requires an action", label)
		}
	}
	return nil
}

// checkMachineStepLabels enforces R-MACH-013: transition labels and
// runtime-seeded labels follow the command-state label grammar, and
// runtime-seeded labels are unique.
func checkMachineStepLabels(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	transitions, err := machTransitions(doc)
	if err != nil {
		return err
	}
	for index, transition := range transitions {
		if raw, present := transition["label"]; present && !machLabel(machString(raw)) {
			return fmt.Errorf("transitions[%d].label %q is not a command-state label", index, machString(raw))
		}
	}
	external, _ := doc["external_labels"].([]any)
	seen := map[string]bool{}
	for index, raw := range external {
		name := machString(raw)
		if entry, ok := raw.(map[string]any); ok {
			name = machString(entry["name"])
		}
		if !machLabel(name) {
			return fmt.Errorf("external_labels[%d] %q is not a command-state label", index, name)
		}
		if seen[name] {
			return fmt.Errorf("external_labels[%d] repeats %q", index, name)
		}
		seen[name] = true
	}
	return nil
}

// checkMachineForEachShape enforces R-MACH-016.
func checkMachineForEachShape(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	states, signals := machNames(doc["states"]), machNames(doc["signals"])
	terminals := machNames(doc["terminal_states"])
	transitions, err := machTransitions(doc)
	if err != nil {
		return err
	}
	routed := map[string]bool{}
	for _, transition := range transitions {
		routed[machString(transition["state"])+"\x00"+machString(transition["signal"])] = true
	}
	for index, transition := range transitions {
		raw, present := transition["for_each"]
		if !present {
			continue
		}
		label := fmt.Sprintf("transitions[%d].for_each", index)
		spec, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be a mapping", label)
		}
		if err := machClosedKeys(label, spec, machForEachKeynames); err != nil {
			return err
		}
		if action := machString(transition["action"]); action == "" || action == "$tool" {
			return fmt.Errorf("%s requires a named action", label)
		}
		if !machFromSelector(machString(spec["items"])) {
			return fmt.Errorf("%s.items is not a $from(label).path selector", label)
		}
		if !machLabel(machString(spec["as"])) {
			return fmt.Errorf("%s.as is not a command-state label", label)
		}
		continueOn, _ := spec["continue_on"].([]any)
		if len(continueOn) == 0 {
			return fmt.Errorf("%s.continue_on names no signal", label)
		}
		abortOn, _ := spec["abort_on"].([]any)
		for _, signal := range append(append([]any{}, continueOn...), abortOn...) {
			if !signals[machString(signal)] {
				return fmt.Errorf("%s names undeclared signal %v", label, signal)
			}
		}
		join, ok := spec["join"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s.join is absent or not a mapping", label)
		}
		if err := machClosedKeys(label+".join", join, machJoinKeynames); err != nil {
			return err
		}
		next := machString(join["next"])
		if !states[next] {
			return fmt.Errorf("%s.join.next %q is not a declared state", label, next)
		}
		if raw, present := join["label"]; present && !machLabel(machString(raw)) {
			return fmt.Errorf("%s.join.label is not a command-state label", label)
		}
		joinSignals, ok := join["signals"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s.join.signals is absent or not a mapping", label)
		}
		if err := machClosedKeys(label+".join.signals", joinSignals, machJoinSignalKeynames); err != nil {
			return err
		}
		for _, outcome := range []string{"all_success", "failed", "empty", "partial"} {
			raw, present := joinSignals[outcome]
			if !present && outcome == "partial" {
				continue
			}
			signal := machString(raw)
			if !signals[signal] {
				return fmt.Errorf("%s.join.signals.%s is absent or not a declared signal", label, outcome)
			}
			if !terminals[next] && !routed[next+"\x00"+signal] {
				return fmt.Errorf("%s.join: no transition routes %q from %q", label, signal, next)
			}
		}
	}
	return nil
}

// checkMachineForEachPolicy enforces R-MACH-017.
func checkMachineForEachPolicy(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	transitions, err := machTransitions(doc)
	if err != nil {
		return err
	}
	for index, transition := range transitions {
		spec, ok := transition["for_each"].(map[string]any)
		if !ok {
			continue
		}
		label := fmt.Sprintf("transitions[%d].for_each", index)
		mode := machString(spec["mode"])
		if mode != "" && mode != "sequential" && mode != "parallel" {
			return fmt.Errorf("%s.mode %q is neither sequential nor parallel", label, mode)
		}
		bound, bounded := spec["max_concurrency"]
		if mode == "parallel" {
			if value, ok := bound.(int); !ok || value <= 0 {
				return fmt.Errorf("%s.max_concurrency must be a positive integer for parallel mode", label)
			}
		} else if bounded {
			if value, ok := bound.(int); !ok || value != 0 {
				return fmt.Errorf("%s.max_concurrency is valid only for parallel mode", label)
			}
		}
		failure := machString(spec["failure"])
		if failure != "" && failure != "fail_fast" && failure != "collect_all" {
			return fmt.Errorf("%s.failure %q is neither fail_fast nor collect_all", label, failure)
		}
		if failure == "collect_all" {
			join, _ := spec["join"].(map[string]any)
			signals, _ := join["signals"].(map[string]any)
			if machString(signals["partial"]) == "" {
				return fmt.Errorf("%s.join.signals.partial is required for collect_all", label)
			}
		}
	}
	return nil
}

// checkModelStageFragment enforces R-MODEL-005.
func checkModelStageFragment(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	if err := machClosedKeys("stage-fragment document", doc, map[string]bool{
		"unit": true, "params": true, "stage": true,
	}); err != nil {
		return err
	}
	if params, _ := doc["params"].([]any); len(params) == 0 {
		return errors.New("a stage-fragment declares no params")
	}
	stage, ok := doc["stage"].(map[string]any)
	if !ok {
		return errors.New("stage is absent or not a mapping")
	}
	return machClosedKeys("stage", stage, map[string]bool{"states": true, "signals": true, "transitions": true})
}

// checkModelMachineTemplate enforces R-MODEL-006.
func checkModelMachineTemplate(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	if err := machClosedKeys("machine-template document", doc, map[string]bool{
		"unit": true, "params": true, "machine": true,
	}); err != nil {
		return err
	}
	if _, ok := doc["params"].([]any); !ok {
		return errors.New("params is absent or not a sequence")
	}
	body, ok := doc["machine"].(map[string]any)
	if !ok {
		return errors.New("machine is absent or not a mapping")
	}
	for _, keyname := range []string{"unit", "imports", "params"} {
		if _, present := body[keyname]; present {
			return fmt.Errorf("the template body carries %q, which belongs to the template file", keyname)
		}
	}
	return machClosedKeys("machine-template body", body, machTopKeynames)
}

var modelParamTypes = map[string]bool{"string": true, "integer": true, "number": true, "boolean": true}

// checkModelFragmentParams enforces R-MODEL-008.
func checkModelFragmentParams(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	params, ok := doc["params"].([]any)
	if !ok {
		return errors.New("params is absent or not a sequence")
	}
	seen := map[string]bool{}
	for index, raw := range params {
		label := fmt.Sprintf("params[%d]", index)
		param, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be a mapping", label)
		}
		if err := machClosedKeys(label, param, map[string]bool{
			"name": true, "type": true, "enum": true, "default": true, "description": true,
		}); err != nil {
			return err
		}
		name := machString(param["name"])
		if name == "" {
			return fmt.Errorf("%s carries no name", label)
		}
		if seen[name] {
			return fmt.Errorf("%s repeats the name %q", label, name)
		}
		seen[name] = true
		kind := machString(param["type"])
		if !modelParamTypes[kind] {
			return fmt.Errorf("%s type %q is not string, integer, number, or boolean", label, kind)
		}
		if raw, present := param["default"]; present {
			enum, _ := param["enum"].([]any)
			if err := modelParamValue(kind, fmt.Sprint(raw), enum); err != nil {
				return fmt.Errorf("%s default: %w", label, err)
			}
		}
	}
	return nil
}

func modelParamValue(kind, value string, enum []any) error {
	switch kind {
	case "integer":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("%q is not an integer", value)
		}
	case "number":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("%q is not a number", value)
		}
	case "boolean":
		if value != "true" && value != "false" {
			return fmt.Errorf("%q is not true or false", value)
		}
	}
	if len(enum) == 0 {
		return nil
	}
	for _, candidate := range enum {
		if fmt.Sprint(candidate) == value {
			return nil
		}
	}
	return fmt.Errorf("%q is not one of the declared enum", value)
}

func machClosedKeys(label string, doc map[string]any, allowed map[string]bool) error {
	for keyname := range doc {
		if !allowed[keyname] {
			return fmt.Errorf("%s carries unknown keyname %q", label, keyname)
		}
	}
	return nil
}

func machTransitions(doc map[string]any) ([]map[string]any, error) {
	raw, ok := doc["transitions"].([]any)
	if !ok {
		return nil, errors.New(`keyname "transitions" is absent or not a sequence`)
	}
	transitions := make([]map[string]any, 0, len(raw))
	for index, entry := range raw {
		transition, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("transitions[%d] must be a mapping", index)
		}
		transitions = append(transitions, transition)
	}
	return transitions, nil
}

// machNames collects the names of a states, signals, or terminal_states list,
// in either the short or the long form.
func machNames(raw any) map[string]bool {
	names := map[string]bool{}
	list, _ := raw.([]any)
	for _, entry := range list {
		switch value := entry.(type) {
		case string:
			names[value] = true
		case map[string]any:
			if name := machString(value["name"]); name != "" {
				names[name] = true
			}
		}
	}
	return names
}

func machString(raw any) string {
	value, _ := raw.(string)
	return value
}

// machLabel is the command-state label grammar: non-empty, untrimmed, free of
// whitespace, control characters, dots, and parentheses.
func machLabel(label string) bool {
	if !machComponent(label) {
		return false
	}
	return !strings.ContainsAny(label, "().")
}

func machComponent(component string) bool {
	if component == "" {
		return false
	}
	for _, r := range component {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func machPath(path string) bool {
	for _, component := range strings.Split(path, ".") {
		if !machComponent(component) {
			return false
		}
	}
	return true
}

func machDotPath(selector string) bool {
	return strings.HasPrefix(selector, "$.") && machPath(strings.TrimPrefix(selector, "$."))
}

func machFromSelector(selector string) bool {
	remainder, ok := strings.CutPrefix(selector, "$from(")
	if !ok {
		return false
	}
	label, after, ok := strings.Cut(remainder, ")")
	if !ok || !machLabel(label) {
		return false
	}
	path, ok := strings.CutPrefix(after, ".")
	return ok && machPath(path)
}
