// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

type dumpDocument struct {
	Version        int                  `yaml:"dump_version"`
	Profile        catalog.AgentProfile `yaml:"profile"`
	Machine        core.MachineSpec     `yaml:"machine"`
	Types          []dumpType           `yaml:"types,omitempty"`
	Instantiations []dumpInstantiation  `yaml:"instantiations,omitempty"`
	Tools          []catalog.ToolDef    `yaml:"tools"`
	Rest           restDump             `yaml:"rest"`
	Files          []dumpFile           `yaml:"files"`
}

// dumpType renders one declared type under its unit.Name address. Tool schemas
// are already resolved by the time they reach the dump, so this section is
// where a reader sees the declared shape a reference pointed at
// (srd051 R5.3).
type dumpType struct {
	Ref         string         `yaml:"ref"`
	Description string         `yaml:"description,omitempty"`
	Schema      map[string]any `yaml:"schema,omitempty"`
}

// dumpInstantiation renders one application of a fragment: where it came
// from, what filled it, and what it produced (srd052 R3.2).
type dumpInstantiation struct {
	Fragment string            `yaml:"fragment"`
	As       string            `yaml:"as,omitempty"`
	Args     map[string]string `yaml:"args,omitempty"`
	Produces []string          `yaml:"produces"`
}

type restDump struct {
	Version          string                             `yaml:"version,omitempty"`
	Clients          map[string]restdef.Client          `yaml:"clients,omitempty"`
	Servers          map[string]restdef.Server          `yaml:"servers,omitempty"`
	Auth             map[string]restdef.AuthProfile     `yaml:"auth,omitempty"`
	Limits           map[string]restdef.LimitProfile    `yaml:"limits,omitempty"`
	RetryPolicies    map[string]restdef.RetryPolicy     `yaml:"retry_policies,omitempty"`
	ResponseMappings map[string]restdef.ResponseMapping `yaml:"response_mappings,omitempty"`
}

type dumpFile struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

// DumpConfig writes the canonical, resolved representation of a closure.
func DumpConfig(closure *Closure, writer io.Writer) error {
	if closure == nil {
		return fmt.Errorf("profile closure is nil")
	}
	tools := append([]catalog.ToolDef(nil), closure.Selected...)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	files, err := dumpFiles(closure)
	if err != nil {
		return err
	}
	data, err := canonicalYAML(dumpDocument{
		Version: 1, Profile: closure.Profile, Machine: closure.Machine,
		Types: newTypeDump(closure.Types), Instantiations: newInstantiationDump(closure.ToolUniverse),
		Tools: tools, Rest: newRestDump(closure.Rest), Files: files,
	})
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// newTypeDump renders every declared type in the registry's sorted address
// order, so a dump is byte-identical across repeated runs.
func newTypeDump(registry *typesys.Registry) []dumpType {
	refs := registry.Refs()
	if len(refs) == 0 {
		return nil
	}
	types := make([]dumpType, 0, len(refs))
	for _, ref := range refs {
		declared, ok := registry.Resolve(ref)
		if !ok {
			continue
		}
		types = append(types, dumpType{
			Ref: ref, Description: declared.Description, Schema: declared.Schema,
		})
	}
	return types
}

// newInstantiationDump groups the universe's tools by the fragment
// application that produced them, sorted by fragment path then prefix, with
// produced names sorted, so a dump is byte-identical across runs.
func newInstantiationDump(universe []catalog.ToolDef) []dumpInstantiation {
	byKey := map[string]*dumpInstantiation{}
	for _, tool := range universe {
		instantiation, ok := tool.Instantiation()
		if !ok {
			continue
		}
		key := instantiation.Fragment + "\x00" + instantiation.As
		entry, exists := byKey[key]
		if !exists {
			entry = &dumpInstantiation{
				Fragment: instantiation.Fragment, As: instantiation.As, Args: instantiation.Args,
			}
			byKey[key] = entry
		}
		entry.Produces = append(entry.Produces, tool.Name)
	}
	if len(byKey) == 0 {
		return nil
	}
	entries := make([]dumpInstantiation, 0, len(byKey))
	for _, entry := range byKey {
		sort.Strings(entry.Produces)
		entries = append(entries, *entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Fragment != entries[j].Fragment {
			return entries[i].Fragment < entries[j].Fragment
		}
		return entries[i].As < entries[j].As
	})
	return entries
}

func dumpFiles(closure *Closure) ([]dumpFile, error) {
	paths := append([]string(nil), closure.Files...)
	sort.Strings(paths)
	files := make([]dumpFile, 0, len(paths))
	for _, path := range paths {
		data, ok := closure.Assets[path]
		if !ok {
			return nil, fmt.Errorf("closure asset %s has no captured bytes", path)
		}
		sum := sha256.Sum256(data)
		files = append(files, dumpFile{Path: path, SHA256: hex.EncodeToString(sum[:])})
	}
	return files, nil
}

func newRestDump(collection toolrest.Collection) restDump {
	return restDump{
		Version: collection.Version, Clients: collection.Clients, Servers: collection.Servers,
		Auth: collection.Auth, Limits: collection.Limits,
		RetryPolicies:    collection.RetryPolicies,
		ResponseMappings: collection.ResponseMappings,
	}
}

func canonicalYAML(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode canonical config dump: %w", err)
	}
	return output.Bytes(), nil
}
