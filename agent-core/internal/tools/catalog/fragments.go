// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
)

// Instantiating tool-declaration fragments (srd052 R2). A fragment is read
// from its expanded bytes, its arguments are substituted into scalar values,
// and the result is fed through the same parse and validation as a
// hand-written unit standing at the fragment's own path, so its imports
// resolve where the fragment author wrote them.

// ToolInstantiation is the fragment application that produced a tool
// (srd052 R3.2).
type ToolInstantiation struct {
	Fragment string
	As       string
	Args     map[string]string
}

// Instantiation returns the fragment a tool was produced from, if any.
func (td ToolDef) Instantiation() (ToolInstantiation, bool) {
	if td.fragment == "" {
		return ToolInstantiation{}, false
	}
	return ToolInstantiation{Fragment: td.fragment, As: td.fragmentAs, Args: td.fragmentArgs}, true
}

func validateFragmentFile(file ToolDefsFile, path string, imported bool) error {
	if !file.IsFragment() {
		return nil
	}
	if imported {
		return fmt.Errorf("fragment %s is instantiated, not imported (srd052 R2.6)", path)
	}
	if file.hasInstantiate {
		return fmt.Errorf("fragment %s instantiates another fragment; nesting is not supported (srd052 R1.2)", path)
	}
	if file.Unit == "" {
		return fmt.Errorf("fragment %s must declare unit", path)
	}
	if err := fragments.ValidateParams(file.Params); err != nil {
		return fmt.Errorf("fragment %s: %w", path, err)
	}
	return nil
}

func (r *toolImportResolver) resolveInstantiations(file ToolDefsFile, path string) ([]ToolDef, error) {
	var all []ToolDef
	producedBy := map[string]int{}
	for index, instantiation := range file.Instantiate {
		defs, err := r.instantiate(ToolSource{Unit: file.Unit, Path: path}, instantiation)
		if err != nil {
			return nil, fmt.Errorf("tool unit %q at %s instantiates %q: %w",
				file.Unit, path, instantiation.Fragment, err)
		}
		for _, def := range defs {
			if previous, seen := producedBy[def.Name]; seen {
				return nil, fmt.Errorf("duplicate imported tool %q: instantiations %d and %d of %s",
					def.Name, previous+1, index+1, instantiation.Fragment)
			}
			producedBy[def.Name] = index
		}
		all = append(all, defs...)
	}
	return all, nil
}

func (r *toolImportResolver) instantiate(
	importer ToolSource, instantiation fragments.Instantiation,
) ([]ToolDef, error) {
	if strings.TrimSpace(instantiation.Fragment) == "" {
		return nil, fmt.Errorf("fragment path must be non-empty")
	}
	target, err := toolImportTarget(importer.Path, instantiation.Fragment)
	if err != nil {
		return nil, fmt.Errorf("fragment path %q: %w", instantiation.Fragment, err)
	}
	fragment, err := r.readFile(target)
	if err != nil {
		return nil, err
	}
	if !fragment.IsFragment() {
		return nil, fmt.Errorf("%s declares no params, so it is imported, not instantiated", target)
	}
	if err := r.validateFile(fragment, target, false); err != nil {
		return nil, err
	}
	args, err := fragments.ResolveArgs(fragment.Params, instantiation.Args)
	if err != nil {
		return nil, fmt.Errorf("fragment %s: %w", target, err)
	}
	instantiated, err := r.substitute(target, args)
	if err != nil {
		return nil, err
	}
	return r.resolveInstantiated(instantiated, fragment, target, importer, instantiation, args)
}

// substitute fills the fragment's expanded bytes and parses the result the
// way a hand-written file is parsed, so strict decoding and every per-tool
// hook run on what the arguments produced (srd052 R2.3, R2.4).
func (r *toolImportResolver) substitute(target string, args map[string]fragments.Arg) (ToolDefsFile, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(r.raw[target], &document); err != nil {
		return ToolDefsFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	if err := fragments.Substitute(&document, args); err != nil {
		return ToolDefsFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	if line, found := fragments.Leftover(&document); found {
		return ToolDefsFile{}, fmt.Errorf("fragment %s: line %d: $param( survives substitution", target, line)
	}
	fragments.RemoveField(&document, "params")
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return ToolDefsFile{}, fmt.Errorf("fragment %s: encode instantiation: %w", target, err)
	}
	instantiated, err := parseToolDefsFileRaw(output.Bytes(), false, false)
	if err != nil {
		return ToolDefsFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	return instantiated, nil
}

func (r *toolImportResolver) resolveInstantiated(
	instantiated, fragment ToolDefsFile, target string, importer ToolSource,
	instantiation fragments.Instantiation, args map[string]fragments.Arg,
) ([]ToolDef, error) {
	values := make(map[string]string, len(args))
	for name, arg := range args {
		values[name] = arg.Value
	}
	source := ToolSource{Unit: fragment.Unit, Path: target}
	local := annotateToolSources(instantiated.Tools, source)
	for index := range local {
		if instantiation.As != "" {
			local[index].Name = instantiation.As + "_" + local[index].Name
		}
		local[index].fragment, local[index].fragmentAs, local[index].fragmentArgs = target, instantiation.As, values
	}
	if err := validateAndDefaultToolDefs(local); err != nil {
		return nil, fmt.Errorf("fragment %s: %w", target, err)
	}
	if err := r.resolveConfigFiles(local, target); err != nil {
		return nil, fmt.Errorf("fragment %s: %w", target, err)
	}
	imported, err := r.resolveImports(instantiated, target)
	if err != nil {
		return nil, err
	}
	r.imports = append(r.imports, ToolImport{Importer: importer, Imported: source, Args: values})
	merged, err := mergeImportedTools(imported, local)
	if err != nil {
		return nil, fmt.Errorf("fragment %s: %w", target, err)
	}
	return merged, nil
}
