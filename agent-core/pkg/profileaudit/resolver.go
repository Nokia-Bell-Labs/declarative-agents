// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profileaudit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	internalload "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

type loadedClosure struct {
	profilePath string
	machinePath string
	defs        []catalog.ToolDef
	rest        toolrest.Collection
	machine     core.MachineSpec
}

func (i *inspector) inspectProfile(profilePath, machineOverride string) error {
	closure, key, err := loadProfileClosure(profilePath, machineOverride)
	if err != nil {
		return err
	}
	if done, err := i.beginVisit(key); done || err != nil {
		return err
	}
	defer delete(i.visiting, key)
	if err := i.inspectLoaded(closure); err != nil {
		return err
	}
	i.visited[key] = true
	return nil
}

func (i *inspector) inspectClosure(closure *internalload.Closure) error {
	loaded := loadedClosure{
		profilePath: closure.ProfilePath,
		machinePath: canonical(closure.Profile.Machine),
		defs:        closure.Selected,
		rest:        closure.Rest,
		machine:     closure.Machine,
	}
	key := loaded.profilePath + "|" + loaded.machinePath
	if done, err := i.beginVisit(key); done || err != nil {
		return err
	}
	defer delete(i.visiting, key)
	if err := i.inspectLoaded(loaded); err != nil {
		return err
	}
	i.visited[key] = true
	return nil
}

func loadProfileClosure(profilePath, machineOverride string) (loadedClosure, string, error) {
	profilePath = resolveReference("", profilePath)
	resolved, err := internalload.LoadClosure(profilePath, internalload.Options{})
	if err != nil {
		return loadedClosure{}, "", fmt.Errorf("inspect profile %s: %w", profilePath, err)
	}
	machinePath := resolved.Profile.Machine
	machine := resolved.Machine
	defs := resolved.Selected
	if machineOverride != "" {
		machinePath = resolveReference(filepath.Dir(profilePath), machineOverride)
		machine, err = core.LoadMachineSpec(machinePath)
		if err != nil {
			return loadedClosure{}, "", fmt.Errorf("inspect profile %s machine: %w", profilePath, err)
		}
		defs, err = loadRequestTools(resolved.Profile, resolved.ToolUniverse, machine)
		if err != nil {
			return loadedClosure{}, "", fmt.Errorf("inspect profile %s tools: %w", profilePath, err)
		}
	}
	closure := loadedClosure{
		profilePath: canonical(profilePath), machinePath: canonical(machinePath),
		defs: defs, rest: resolved.Rest, machine: machine,
	}
	return closure, closure.profilePath + "|" + closure.machinePath, nil
}

func (i *inspector) beginVisit(key string) (bool, error) {
	if i.visited[key] {
		return true, nil
	}
	if i.visiting[key] {
		return false, fmt.Errorf("profile timeout closure cycle at %s", key)
	}
	i.visiting[key] = true
	return false, nil
}

func loadPointTools(dirs, declarations, selections []string) ([]catalog.ToolDef, error) {
	fromDirs, err := catalog.LoadToolDeclarationsFromDirs(dirs)
	if err != nil {
		return nil, err
	}
	explicit, err := catalog.LoadToolDeclarations(declarations)
	if err != nil {
		return nil, err
	}
	names, err := catalog.LoadToolSelections(selections)
	if err != nil {
		return nil, err
	}
	return catalog.SelectTools(catalog.MergeToolDefs(fromDirs, explicit), names)
}

// loadRequestTools mirrors ProfileMachineRequestRunner: a machine override
// selects its literal actions from all profile declarations, while dynamic
// vocabulary remains restricted by the profile's ordinary selection.
func loadRequestTools(
	profile catalog.AgentProfile, universe []catalog.ToolDef, machine core.MachineSpec,
) ([]catalog.ToolDef, error) {
	names, err := requestActionNames(machine, profile.Tools, universe)
	if err != nil {
		return nil, err
	}
	return catalog.SelectTools(universe, names)
}

func requestActionNames(
	machine core.MachineSpec, selections []string, defs []catalog.ToolDef,
) ([]string, error) {
	selected := make(map[string]bool)
	for _, transition := range machine.Transitions {
		if transition.Action != "" && transition.Action != "$tool" {
			selected[transition.Action] = true
		}
	}
	if machineUsesDynamicAction(machine) {
		names, err := catalog.LoadToolSelections(selections)
		if err != nil {
			return nil, err
		}
		addDynamicActions(selected, names, defs)
	}
	return sortedNames(selected), nil
}

func addDynamicActions(selected map[string]bool, names []string, defs []catalog.ToolDef) {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	for _, def := range defs {
		if def.Visibility != "internal" && allowed[def.Name] {
			selected[def.Name] = true
		}
	}
}

func sortedNames(selected map[string]bool) []string {
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func machineUsesDynamicAction(machine core.MachineSpec) bool {
	for _, transition := range machine.Transitions {
		if transition.Action == "$tool" {
			return true
		}
	}
	return false
}

func (i *inspector) inspectLoaded(closure loadedClosure) error {
	machine := closure.machine
	commandRaw := ""
	if machine.BudgetSpec != nil {
		commandRaw = machine.BudgetSpec.CommandTimeout
	}
	commandTimeout, commandErr := time.ParseDuration(commandRaw)
	if commandErr != nil || commandTimeout <= 0 {
		i.addInvalid(closure, "", "budget.command_timeout", commandRaw, 0,
			"command_timeout must be a finite positive duration")
		return nil
	}

	defs := make(map[string]catalog.ToolDef, len(closure.defs))
	for _, def := range closure.defs {
		defs[def.Name] = def
	}
	for _, action := range reachableActions(machine, closure.defs) {
		def, ok := defs[action]
		if !ok {
			return fmt.Errorf(
				"inspect profile %s machine %s: reachable action %q is not selected",
				closure.profilePath, closure.machinePath, action,
			)
		}
		if err := i.inspectAction(closure, commandTimeout, def); err != nil {
			return err
		}
	}
	return nil
}

func reachableActions(machine core.MachineSpec, defs []catalog.ToolDef) []string {
	reachable := reachableStates(machine)
	actions, dynamic := actionsFromStates(machine, reachable)
	if dynamic {
		for _, def := range defs {
			if def.Visibility != "internal" {
				actions[def.Name] = true
			}
		}
	}
	return sortedNames(actions)
}

func reachableStates(machine core.MachineSpec) map[string]bool {
	reachable := map[string]bool{machine.InitialState: true}
	for changed := true; changed; {
		changed = false
		for _, transition := range machine.Transitions {
			if !reachable[transition.State] {
				continue
			}
			for _, next := range []string{transition.Next, forEachNext(transition)} {
				if next != "" && !reachable[next] {
					reachable[next], changed = true, true
				}
			}
		}
	}
	return reachable
}

func actionsFromStates(machine core.MachineSpec, reachable map[string]bool) (map[string]bool, bool) {
	actions := make(map[string]bool)
	dynamic := false
	for _, transition := range machine.Transitions {
		if !reachable[transition.State] {
			continue
		}
		switch transition.Action {
		case "":
		case "$tool":
			dynamic = true
		default:
			actions[transition.Action] = true
		}
	}
	return actions, dynamic
}

func forEachNext(transition core.TransitionSpec) string {
	if transition.ForEach == nil {
		return ""
	}
	return transition.ForEach.Join.Next
}

func (i *inspector) inspectChildProfile(closure loadedClosure, def catalog.ToolDef, field string) error {
	child, ok := configString(def.Config, field)
	if !ok || strings.TrimSpace(child) == "" {
		return fmt.Errorf("profile %s action %q requires config.%s", closure.profilePath, def.Name, field)
	}
	return i.inspectProfile(resolveReference(filepath.Dir(closure.profilePath), child), "")
}

func (i *inspector) inspectPointMachine(closure loadedClosure, def catalog.ToolDef) error {
	point, err := loadPointClosure(closure, def)
	if err != nil {
		return err
	}
	key := point.profilePath + "|" + canonical(point.machinePath)
	if i.visited[key] {
		return nil
	}
	if i.visiting[key] {
		return fmt.Errorf("profile timeout closure cycle at %s", key)
	}
	i.visiting[key] = true
	defer delete(i.visiting, key)
	if err := i.inspectLoaded(point); err != nil {
		return err
	}
	i.visited[key] = true
	return nil
}

func loadPointClosure(closure loadedClosure, def catalog.ToolDef) (loadedClosure, error) {
	machineRef, machineOK := configString(def.Config, "point_machine")
	toolsRef, toolsOK := configString(def.Config, "point_tools")
	declarationValues, declarationsOK := configStrings(def.Config["point_tool_declarations"])
	if !machineOK || !toolsOK || !declarationsOK {
		return loadedClosure{}, fmt.Errorf(
			"profile %s action %q has incomplete evaluator point configuration",
			closure.profilePath, def.Name,
		)
	}
	base := filepath.Dir(closure.profilePath)
	declarations := make([]string, len(declarationValues))
	for n, path := range declarationValues {
		declarations[n] = resolveReference(base, path)
	}
	defs, err := loadPointTools(nil, declarations, []string{resolveReference(base, toolsRef)})
	if err != nil {
		return loadedClosure{}, fmt.Errorf("inspect evaluator point action %q: %w", def.Name, err)
	}
	point := loadedClosure{
		profilePath: closure.profilePath,
		machinePath: resolveReference(base, machineRef),
		defs:        defs,
		rest:        closure.rest,
	}
	point.machine, err = core.LoadMachineSpec(point.machinePath)
	if err != nil {
		return loadedClosure{}, fmt.Errorf("inspect evaluator point action %q machine: %w", def.Name, err)
	}
	return point, nil
}

func resolveReference(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return canonical(catalog.ResolveConfiguredPath("", path))
	}
	if base == "" {
		return canonical(path)
	}
	for dir := base; dir != ""; dir = filepath.Dir(dir) {
		candidate := catalog.ResolveConfiguredPath(dir, path)
		if _, err := os.Stat(candidate); err == nil {
			return canonical(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return canonical(catalog.ResolveConfiguredPath(base, path))
}

func canonical(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}
