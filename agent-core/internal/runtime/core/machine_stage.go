// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/yamlstrict"
)

// Machine stage fragments (srd052 R4). A stage is a fragment whose body
// declares states, signals, and transitions; a machine instantiates it with
// arguments, and the loader splices the result into the machine's own lists
// before validating the whole as one machine. Every state a spliced
// transition names that the stage does not itself declare must already be
// the machine's, so the stage is wired by the importer and checked by the
// same rules as a hand-written transition (srd052 R4.2).

// StageSpec is the body of a stage fragment.
type StageSpec struct {
	States      StateSpecs       `yaml:"states,omitempty"`
	Signals     SignalSpecs      `yaml:"signals,omitempty"`
	Transitions []TransitionSpec `yaml:"transitions"`
}

// MachineInstantiation records one fragment a machine instantiated, for the
// dump (srd052 R3.2): a stage spliced into it, or the template it instantiates.
type MachineInstantiation struct {
	Kind     string
	Fragment string
	As       string
	Args     map[string]string
	Produces []string
}

// Instantiation kinds a machine records (srd054 R3.2).
const (
	InstantiationKindStage   = "stage"
	InstantiationKindMachine = "machine"
)

// Instantiations returns the stage fragments spliced into the machine.
func (m MachineSpec) Instantiations() []MachineInstantiation {
	return append([]MachineInstantiation(nil), m.instantiations...)
}

// LoadMachineClosure reads a machine file and validates it as one machine. A
// machine that instantiates a machine template is replaced by the template's
// instantiation (srd054); a machine that instantiates stage fragments has them
// spliced in (srd052 R4). visit, when set, sees the machine file and each
// fragment file, so a closure records them.
func LoadMachineClosure(path string, visit func(string, []byte) error) (MachineSpec, error) {
	data, err := readMachineFile(path, visit)
	if err != nil {
		return MachineSpec{}, err
	}
	if kind, kindErr := fragmentBodyKind(data); kindErr == nil && kind == InstantiationKindMachine {
		return MachineSpec{}, fmt.Errorf("machine spec %s is a machine template: a template is "+
			"instantiated by a machine file, not loaded as a machine (srd054 R1.4)", path)
	}
	spec, err := decodeMachineSpec(data)
	if err != nil {
		return MachineSpec{}, fmt.Errorf("parse machine spec %s: %w", path, err)
	}
	if len(spec.Imports) > 0 {
		return MachineSpec{}, fmt.Errorf(
			"machine spec %s imports %q: a machine imports nothing; a stage fragment is instantiated", path, spec.Imports)
	}
	template, err := instantiatedTemplate(path, spec.Instantiate)
	if err != nil {
		return MachineSpec{}, fmt.Errorf("machine spec %s: %w", path, err)
	}
	if template != "" {
		return loadMachineInstance(path, data, spec.Instantiate[0], template, visit)
	}
	if err := spliceStages(&spec, path, visit); err != nil {
		return MachineSpec{}, err
	}
	if err := validateSpec(spec); err != nil {
		if len(spec.instantiations) > 0 {
			return MachineSpec{}, fmt.Errorf("machine spec %s after splicing %s: %w",
				path, strings.Join(splicedFragments(spec), ", "), err)
		}
		return MachineSpec{}, fmt.Errorf("parse machine spec %s: %w", path, err)
	}
	return spec, nil
}

// spliceStages splices every stage fragment spec instantiates, resolving their
// paths against the file at base, and clears the instantiation list.
func spliceStages(spec *MachineSpec, base string, visit func(string, []byte) error) error {
	for _, instantiation := range spec.Instantiate {
		if err := spliceStageFragment(spec, base, instantiation, visit); err != nil {
			return fmt.Errorf("machine spec %s instantiates %q: %w", base, instantiation.Fragment, err)
		}
	}
	spec.Instantiate = nil
	return nil
}

func readMachineFile(path string, visit func(string, []byte) error) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read machine spec %s: %w", path, err)
	}
	if visit != nil {
		if err := visit(path, data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// stageHeader is what a stage fragment declares before its arguments arrive.
// The body stays a node: it holds $param references where typed fields stand.
type stageHeader struct {
	Unit   string            `yaml:"unit"`
	Params []fragments.Param `yaml:"params"`
	Stage  yaml.Node         `yaml:"stage"`
}

type stageFile struct {
	Unit  string    `yaml:"unit"`
	Stage StageSpec `yaml:"stage"`
}

func spliceStageFragment(
	spec *MachineSpec, machinePath string, instantiation fragments.Instantiation,
	visit func(string, []byte) error,
) error {
	if strings.TrimSpace(instantiation.Fragment) == "" {
		return fmt.Errorf("fragment path must be non-empty")
	}
	resolved, err := corepath.ImportTarget(machinePath, instantiation.Fragment)
	if err != nil {
		return fmt.Errorf("fragment path %q: %w", instantiation.Fragment, err)
	}
	target := filepath.Clean(resolved)
	header, data, err := readStageHeader(target, visit)
	if err != nil {
		return err
	}
	args, err := fragments.ResolveArgs(header.Params, instantiation.Args)
	if err != nil {
		return fmt.Errorf("fragment %s: %w", target, err)
	}
	stage, err := instantiateStage(data, target, args)
	if err != nil {
		return err
	}
	values := make(map[string]string, len(args))
	for name, arg := range args {
		values[name] = arg.Value
	}
	spec.States = append(spec.States, stage.Stage.States...)
	spec.Signals = append(spec.Signals, stage.Stage.Signals...)
	spec.Transitions = append(spec.Transitions, stage.Stage.Transitions...)
	spec.instantiations = append(spec.instantiations, MachineInstantiation{
		Kind: InstantiationKindStage, Fragment: target, As: instantiation.As, Args: values,
		Produces: stageProduces(stage.Stage),
	})
	return nil
}

// readStageHeader reads a stage fragment and decodes only its header: the
// body holds $param references where typed fields stand, so it is decoded
// after substitution (srd052 R2.4).
func readStageHeader(target string, visit func(string, []byte) error) (stageHeader, []byte, error) {
	data, err := readMachineFile(target, visit)
	if err != nil {
		return stageHeader{}, nil, err
	}
	var header stageHeader
	if err := yamlstrict.Unmarshal(data, &header); err != nil {
		return stageHeader{}, nil, fmt.Errorf("fragment %s: %w", target, err)
	}
	if len(header.Params) == 0 || header.Stage.Kind == 0 {
		return stageHeader{}, nil, fmt.Errorf("fragment %s: a stage fragment declares params and a stage body", target)
	}
	if err := fragments.ValidateParams(header.Params); err != nil {
		return stageHeader{}, nil, fmt.Errorf("fragment %s: %w", target, err)
	}
	return header, data, nil
}

// instantiateStage fills the fragment's bytes and decodes the result strictly,
// the way a hand-written machine is decoded (srd052 R2.3, R2.4).
func instantiateStage(data []byte, target string, args map[string]fragments.Arg) (stageFile, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return stageFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	if err := fragments.Substitute(&document, args); err != nil {
		return stageFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	if line, found := fragments.Leftover(&document); found {
		return stageFile{}, fmt.Errorf("fragment %s: line %d: $param( survives substitution", target, line)
	}
	fragments.RemoveField(&document, "params")
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return stageFile{}, fmt.Errorf("fragment %s: encode instantiation: %w", target, err)
	}
	var stage stageFile
	if err := yamlstrict.Unmarshal(output.Bytes(), &stage); err != nil {
		return stageFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	return stage, nil
}

func stageProduces(stage StageSpec) []string {
	var names []string
	for _, state := range stage.States {
		names = append(names, "states/"+state.Name)
	}
	for _, signal := range stage.Signals {
		names = append(names, "signals/"+signal.Name)
	}
	for _, transition := range stage.Transitions {
		names = append(names, "transitions/"+transition.State+":"+transition.Signal)
	}
	sort.Strings(names)
	return names
}

func splicedFragments(spec MachineSpec) []string {
	paths := make([]string, 0, len(spec.instantiations))
	for _, instantiation := range spec.instantiations {
		paths = append(paths, instantiation.Fragment)
	}
	return paths
}
