// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/yamlstrict"
)

// Machine templates (srd054). A template is a fragment whose body is a whole
// machine; a machine file that instantiates one is an instance, carrying only
// its unit, the instantiation, and optionally its own name and purpose. The
// loader fills the template's arguments, splices any stages the template
// instantiates, applies the instance's name and purpose, and validates the
// result as one hand-written machine.

// instanceFields are the only top-level fields an instance may carry
// (srd054 R1.2).
var instanceFields = []string{"unit", "instantiate", "name", "purpose"}

// templateHeader is what a template declares before its arguments arrive.
type templateHeader struct {
	Unit    string            `yaml:"unit"`
	Params  []fragments.Param `yaml:"params"`
	Machine yaml.Node         `yaml:"machine"`
}

// TemplatePath names the machine template this machine was instantiated from,
// or is empty for a hand-written machine, so a diagnostic raised after loading
// can name the template beside the instance (srd054 R2.5).
func (m MachineSpec) TemplatePath() string {
	for _, instantiation := range m.instantiations {
		if instantiation.Kind == InstantiationKindMachine {
			return instantiation.Fragment
		}
	}
	return ""
}

// DescribeMachine names a machine for a diagnostic: its file, and the template
// it was instantiated from when it is an instance, so a fault found after
// loading points at the file that holds the fault (srd054 R2.5).
func DescribeMachine(path string, spec MachineSpec) string {
	if template := spec.TemplatePath(); template != "" {
		return path + " (instance of machine template " + template + ")"
	}
	return path
}

// fragmentBodyKind reads a fragment's top-level fields and names its body:
// machine, stage, tools, rest, or profile. A file with none of them names none.
func fragmentBodyKind(data []byte) (string, error) {
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return "", err
	}
	if _, ok := fields["params"]; !ok {
		return "none", nil
	}
	for _, kind := range []string{InstantiationKindMachine, InstantiationKindStage, "tools", "rest", "profile"} {
		if _, ok := fields[kind]; ok {
			return kind, nil
		}
	}
	return "none", nil
}

// instantiatedTemplate returns the resolved path of the machine template an
// instantiation list names, or empty when every entry is a stage. A template
// must be the list's only entry, and a target that is neither a stage nor a
// machine template is refused by its body kind (srd054 R1.3, R2.3).
func instantiatedTemplate(path string, instantiations []fragments.Instantiation) (string, error) {
	template := ""
	for _, instantiation := range instantiations {
		target, kind, err := instantiationTarget(path, instantiation)
		if err != nil {
			return "", err
		}
		switch kind {
		case InstantiationKindStage, "none":
			// A file declaring no body is refused by the stage splicer under
			// srd052 R1.3, which names what a stage fragment must declare.
		case InstantiationKindMachine:
			template = target
		default:
			return "", fmt.Errorf("instantiates %s, whose body kind %q is neither a stage nor a machine",
				target, kind)
		}
	}
	if template != "" && len(instantiations) != 1 {
		return "", fmt.Errorf("instantiates machine template %s beside another fragment: an instance "+
			"instantiates exactly one template and splices no stage beside it (srd054 R1.2, R1.3)", template)
	}
	return template, nil
}

func instantiationTarget(base string, instantiation fragments.Instantiation) (string, string, error) {
	target, err := fragmentTarget(base, instantiation.Fragment)
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", "", fmt.Errorf("read fragment %s: %w", target, err)
	}
	kind, err := fragmentBodyKind(data)
	if err != nil {
		return "", "", fmt.Errorf("fragment %s: %w", target, err)
	}
	return target, kind, nil
}

// loadMachineInstance loads the machine an instance file names: the template
// instantiated with the instance's arguments, its stages spliced, and the
// instance's name and purpose applied (srd054 R1, R2).
func loadMachineInstance(
	path string, data []byte, instantiation fragments.Instantiation, template string,
	visit func(string, []byte) error,
) (MachineSpec, error) {
	if err := checkInstance(path, data, instantiation); err != nil {
		return MachineSpec{}, err
	}
	var instance MachineSpec
	if err := yamlstrict.Unmarshal(data, &instance); err != nil {
		return MachineSpec{}, fmt.Errorf("parse machine spec %s: %w", path, err)
	}
	spec, args, err := instantiateTemplate(template, instantiation, visit)
	if err != nil {
		return MachineSpec{}, fmt.Errorf("machine spec %s instantiates template %s: %w", path, template, err)
	}
	if instance.Name != "" {
		spec.Name = instance.Name
	}
	if instance.Purpose != "" {
		spec.Purpose = instance.Purpose
	}
	spec.instantiations = append([]MachineInstantiation{{
		Kind: InstantiationKindMachine, Fragment: template, Args: args,
		Produces: []string{"machine/" + spec.Name},
	}}, spec.instantiations...)
	if err := validateSpec(spec); err != nil {
		return MachineSpec{}, fmt.Errorf("machine spec %s instantiating template %s: %w", path, template, err)
	}
	return spec, nil
}

// checkInstance holds an instance file to its header: only unit, one
// instantiation without a prefix, name, and purpose (srd054 R1.2).
func checkInstance(path string, data []byte, instantiation fragments.Instantiation) error {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse machine spec %s: %w", path, err)
	}
	if len(document.Content) > 0 {
		if err := yamlstrict.CheckFields(document.Content[0], instanceFields...); err != nil {
			return fmt.Errorf("machine spec %s instantiates a machine template, so it carries only %s: %w",
				path, strings.Join(instanceFields, ", "), err)
		}
	}
	if instantiation.As != "" {
		return fmt.Errorf("machine spec %s instantiates a machine template with as %q: an instance "+
			"produces one machine, so there is nothing to prefix (srd054 R1.2)", path, instantiation.As)
	}
	return nil
}

// instantiateTemplate reads a template through visit, fills its arguments into
// the text, decodes its machine body strictly, and splices the stages the body
// instantiates, resolving their paths against the template (srd054 R2.1, R2.2).
func instantiateTemplate(
	template string, instantiation fragments.Instantiation, visit func(string, []byte) error,
) (MachineSpec, map[string]string, error) {
	data, err := readMachineFile(template, visit)
	if err != nil {
		return MachineSpec{}, nil, err
	}
	var header templateHeader
	if err := yamlstrict.Unmarshal(data, &header); err != nil {
		return MachineSpec{}, nil, err
	}
	if err := fragments.ValidateParams(header.Params); err != nil {
		return MachineSpec{}, nil, err
	}
	args, err := fragments.ResolveArgs(header.Params, instantiation.Args)
	if err != nil {
		return MachineSpec{}, nil, err
	}
	spec, err := decodeTemplateBody(data, template, args)
	if err != nil {
		return MachineSpec{}, nil, err
	}
	if err := spliceTemplateStages(&spec, template, visit); err != nil {
		return MachineSpec{}, nil, err
	}
	return spec, argumentValues(args), nil
}

func decodeTemplateBody(data []byte, template string, args map[string]fragments.Arg) (MachineSpec, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return MachineSpec{}, err
	}
	if err := fragments.Substitute(&document, args); err != nil {
		return MachineSpec{}, err
	}
	if line, found := fragments.Leftover(&document); found {
		return MachineSpec{}, fmt.Errorf("line %d: $param( survives substitution", line)
	}
	var instantiated templateHeader
	if err := document.Decode(&instantiated); err != nil {
		return MachineSpec{}, err
	}
	var body bytes.Buffer
	encoder := yaml.NewEncoder(&body)
	encoder.SetIndent(2)
	if err := encoder.Encode(&instantiated.Machine); err != nil {
		return MachineSpec{}, fmt.Errorf("encode instantiation: %w", err)
	}
	spec, err := decodeMachineSpec(body.Bytes())
	if err != nil {
		return MachineSpec{}, fmt.Errorf("machine body: %w", err)
	}
	if spec.Unit != "" || len(spec.Imports) > 0 {
		return MachineSpec{}, fmt.Errorf("machine body carries unit or imports, which belong to the template file")
	}
	return spec, nil
}

// spliceTemplateStages splices the stages a template body instantiates. A
// template instantiating another template is refused: nesting stops at one
// level (srd054 R1.3).
func spliceTemplateStages(spec *MachineSpec, template string, visit func(string, []byte) error) error {
	for _, instantiation := range spec.Instantiate {
		target, kind, err := instantiationTarget(template, instantiation)
		if err != nil {
			return err
		}
		if kind == InstantiationKindMachine {
			return fmt.Errorf("instantiates machine template %s: a template splices stages and "+
				"instantiates no template (srd054 R1.3)", target)
		}
	}
	return spliceStages(spec, template, visit)
}

func argumentValues(args map[string]fragments.Arg) map[string]string {
	values := make(map[string]string, len(args))
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		values[name] = args[name].Value
	}
	return values
}
