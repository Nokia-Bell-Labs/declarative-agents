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
)

type dumpDocument struct {
	Version int                  `yaml:"dump_version"`
	Profile catalog.AgentProfile `yaml:"profile"`
	Machine core.MachineSpec     `yaml:"machine"`
	Tools   []catalog.ToolDef    `yaml:"tools"`
	Rest    restDump             `yaml:"rest"`
	Files   []dumpFile           `yaml:"files"`
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
		Tools: tools, Rest: newRestDump(closure.Rest), Files: files,
	})
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
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
