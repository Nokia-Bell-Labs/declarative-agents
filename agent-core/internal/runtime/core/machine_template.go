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
// machine; a machine file that expands one is an instance, carrying only
// its unit, the expansion, and optionally its own name and purpose. The
// loader fills the template's arguments, splices any stages the template
// expands, applies the instance's name and purpose, and validates the
// result as one hand-written machine.

// instanceFields are the only top-level fields an instance may carry
// (srd054 R1.2).
var instanceFields = []string{"unit", "expand", "name", "purpose"}

// templateHeader is what a template declares before its arguments arrive.
type templateHeader struct {
	Unit    string            `yaml:"unit"`
	Params  []fragments.Param `yaml:"params"`
	Machine yaml.Node         `yaml:"machine"`
}

// TemplatePath names the machine template this machine was expanded from,
// or is empty for a hand-written machine, so a diagnostic raised after loading
// can name the template beside the instance (srd054 R2.5).
func (m MachineSpec) TemplatePath() string {
	for _, expansion := range m.expansions {
		if expansion.Kind == ExpansionKindMachine {
			return expansion.Fragment
		}
	}
	return ""
}

// DescribeMachine names a machine for a diagnostic: its file, and the template
// it was expanded from when it is an instance, so a fault found after
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
	for _, kind := range []string{ExpansionKindMachine, ExpansionKindStage, "tools", "rest", "profile"} {
		if _, ok := fields[kind]; ok {
			return kind, nil
		}
	}
	return "none", nil
}

// expandedTemplate returns the resolved path of the machine template an
// expansion list names, or empty when every entry is a stage. A template
// must be the list's only entry, and a target that is neither a stage nor a
// machine template is refused by its body kind (srd054 R1.3, R2.3).
func expandedTemplate(path string, expansions []fragments.Expansion) (string, error) {
	template := ""
	for _, expansion := range expansions {
		target, kind, err := expansionTarget(path, expansion)
		if err != nil {
			return "", err
		}
		switch kind {
		case ExpansionKindStage, "none":
			// A file declaring no body is refused by the stage splicer under
			// srd052 R1.3, which names what a stage fragment must declare.
		case ExpansionKindMachine:
			template = target
		default:
			return "", fmt.Errorf("expands %s, whose body kind %q is neither a stage nor a machine",
				target, kind)
		}
	}
	if template != "" && len(expansions) != 1 {
		return "", fmt.Errorf("expands machine template %s beside another fragment: an instance "+
			"expands exactly one template and splices no stage beside it (srd054 R1.2, R1.3)", template)
	}
	return template, nil
}

func expansionTarget(base string, expansion fragments.Expansion) (string, string, error) {
	target, err := fragmentTarget(base, expansion.Fragment)
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
// expanded with the instance's arguments, its stages spliced, and the
// instance's name and purpose applied (srd054 R1, R2).
func loadMachineInstance(
	path string, data []byte, expansion fragments.Expansion, template string,
	visit func(string, []byte) error,
) (MachineSpec, error) {
	if err := checkInstance(path, data, expansion); err != nil {
		return MachineSpec{}, err
	}
	var instance MachineSpec
	if err := yamlstrict.Unmarshal(data, &instance); err != nil {
		return MachineSpec{}, fmt.Errorf("parse machine spec %s: %w", path, err)
	}
	spec, args, err := expandTemplate(template, expansion, visit)
	if err != nil {
		return MachineSpec{}, fmt.Errorf("machine spec %s expands template %s: %w", path, template, err)
	}
	if instance.Name != "" {
		spec.Name = instance.Name
	}
	if instance.Purpose != "" {
		spec.Purpose = instance.Purpose
	}
	spec.expansions = append([]MachineExpansion{{
		Kind: ExpansionKindMachine, Fragment: template, Args: args,
		Produces: []string{"machine/" + spec.Name},
	}}, spec.expansions...)
	if err := validateSpec(spec); err != nil {
		return MachineSpec{}, fmt.Errorf("machine spec %s expanding template %s: %w", path, template, err)
	}
	return spec, nil
}

// checkInstance holds an instance file to its header: only unit, one
// expansion without a prefix, name, and purpose (srd054 R1.2).
func checkInstance(path string, data []byte, expansion fragments.Expansion) error {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse machine spec %s: %w", path, err)
	}
	if len(document.Content) > 0 {
		if err := yamlstrict.CheckFields(document.Content[0], instanceFields...); err != nil {
			return fmt.Errorf("machine spec %s expands a machine template, so it carries only %s: %w",
				path, strings.Join(instanceFields, ", "), err)
		}
	}
	if expansion.As != "" {
		return fmt.Errorf("machine spec %s expands a machine template with as %q: an instance "+
			"produces one machine, so there is nothing to prefix (srd054 R1.2)", path, expansion.As)
	}
	return nil
}

// expandTemplate reads a template through visit, fills its arguments into
// the text, decodes its machine body strictly, and splices the stages the body
// expands, resolving their paths against the template (srd054 R2.1, R2.2).
func expandTemplate(
	template string, expansion fragments.Expansion, visit func(string, []byte) error,
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
	args, err := fragments.ResolveArgs(header.Params, expansion.Args)
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
	var expanded templateHeader
	if err := document.Decode(&expanded); err != nil {
		return MachineSpec{}, err
	}
	var body bytes.Buffer
	encoder := yaml.NewEncoder(&body)
	encoder.SetIndent(2)
	if err := encoder.Encode(&expanded.Machine); err != nil {
		return MachineSpec{}, fmt.Errorf("encode expansion: %w", err)
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

// spliceTemplateStages splices the stages a template body expands. A
// template expanding another template is refused: nesting stops at one
// level (srd054 R1.3).
func spliceTemplateStages(spec *MachineSpec, template string, visit func(string, []byte) error) error {
	for _, expansion := range spec.Expand {
		target, kind, err := expansionTarget(template, expansion)
		if err != nil {
			return err
		}
		if kind == ExpansionKindMachine {
			return fmt.Errorf("expands machine template %s: a template splices stages and "+
				"expands no template (srd054 R1.3)", target)
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
