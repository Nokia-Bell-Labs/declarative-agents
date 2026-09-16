// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
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
	// Kind is what the fragment's body is: tool, rest, stage, or machine
	// (srd054 R3.2).
	Kind     string            `yaml:"kind"`
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
	// Root names the library root a file sits under, so a reader can tell a
	// library file from one the profile's own tree supplies (srd056 R3.2).
	Root string `yaml:"root,omitempty"`
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
		Types:          newTypeDump(closure.Types),
		Instantiations: newInstantiationDump(closure.ToolUniverse, closure.Rest, closure.Machine),
		Tools:          tools, Rest: newRestDump(closure.Rest), Files: files,
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
func newInstantiationDump(
	universe []catalog.ToolDef, rest toolrest.Collection, machine core.MachineSpec,
) []dumpInstantiation {
	byKey := map[string]*dumpInstantiation{}
	record := func(kind, fragment, as string, args map[string]string, produced ...string) {
		key := fragment + "\x00" + as
		entry, exists := byKey[key]
		if !exists {
			entry = &dumpInstantiation{Kind: kind, Fragment: fragment, As: as, Args: args}
			byKey[key] = entry
		}
		entry.Produces = append(entry.Produces, produced...)
	}
	for _, tool := range universe {
		if instantiation, ok := tool.Instantiation(); ok {
			record(instantiationKindTool, instantiation.Fragment, instantiation.As, instantiation.Args, tool.Name)
		}
	}
	for _, instantiation := range rest.DeclarationInstantiations() {
		record(instantiationKindREST, instantiation.Fragment, instantiation.As, instantiation.Args,
			instantiation.Produces...)
	}
	for _, instantiation := range machine.Instantiations() {
		record(instantiation.Kind, instantiation.Fragment, instantiation.As, instantiation.Args,
			instantiation.Produces...)
	}
	return sortedInstantiations(byKey)
}

// sortedInstantiations orders rows by fragment path then prefix, with produced
// names sorted, so a dump is byte-identical across runs.
func sortedInstantiations(byKey map[string]*dumpInstantiation) []dumpInstantiation {
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
		files = append(files, dumpFile{
			Path: path, SHA256: hex.EncodeToString(sum[:]), Root: libraryRoot(path),
		})
	}
	return files, nil
}

// libraryRoot names the library a closure file belongs to: agent-core for a
// file in agent-core's library, which the runtime image installs at
// /opt/agent-core/tools and a checkout maps through its install root. Other
// files under the install root, such as integration fixtures, are not library.
func libraryRoot(path string) string {
	if within(corepath.InstallPrefix+"/"+agentCoreLibraryDir, filepath.ToSlash(path)) {
		return agentCoreRoot
	}
	if root := corepath.InstallRoot(); root != "" && within(filepath.Join(root, agentCoreLibraryDir), path) {
		return agentCoreRoot
	}
	return ""
}

func within(dir, path string) bool {
	relative, err := filepath.Rel(dir, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

const (
	instantiationKindTool = "tool"
	instantiationKindREST = "rest"
)

const (
	agentCoreRoot       = "agent-core"
	agentCoreLibraryDir = "tools"
)

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
