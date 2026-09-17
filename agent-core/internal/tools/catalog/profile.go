// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/yamlstrict"
)

// AgentProfile bundles all configuration an agent needs into a single file.
type AgentProfile struct {
	Name             string   `yaml:"name"`
	Machine          string   `yaml:"machine"`
	Tools            []string `yaml:"tools"`
	ToolDeclarations []string `yaml:"tool_declarations"`
	ToolConfigDirs   []string `yaml:"tool_config_dirs,omitempty"`
	RestDefinitions  []string `yaml:"rest_definitions,omitempty"`
	RestConfigDirs   []string `yaml:"rest_config_dirs,omitempty"`
	Directory        string   `yaml:"directory,omitempty"`
	// Libraries declares named library roots: an import under /opt/<name>
	// anywhere in this profile's closure maps to the directory, resolved
	// against this file (srd056 R2.1).
	Libraries map[string]string `yaml:"libraries,omitempty"`
}

// LoadProfile reads a profile YAML file and resolves relative paths.
func LoadProfile(path string) (AgentProfile, error) {
	return LoadProfileWithVisitor(path, nil)
}

// LoadProfileWithVisitor reads a profile and reports its immutable source.
func LoadProfileWithVisitor(path string, visit FileVisitor) (AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentProfile{}, fmt.Errorf("load profile %s: %w", path, err)
	}
	if visit != nil {
		if err := visit(path, data); err != nil {
			return AgentProfile{}, fmt.Errorf("visit profile %s: %w", path, err)
		}
	}
	p, err := parseProfile(path, data)
	if err != nil {
		return AgentProfile{}, err
	}
	resolveProfilePaths(&p, filepath.Dir(path))
	return p, nil
}

func parseProfile(path string, data []byte) (AgentProfile, error) {
	var p AgentProfile
	if err := yamlstrict.Unmarshal(data, &p); err != nil {
		return AgentProfile{}, fmt.Errorf("parse profile %s: %w", path, err)
	}
	if p.Machine == "" {
		return AgentProfile{}, fmt.Errorf("profile %s: machine is required", path)
	}
	if len(p.Tools) == 0 {
		return AgentProfile{}, fmt.Errorf("profile %s: at least one tools entry is required", path)
	}
	for name, directory := range p.Libraries {
		if err := corepath.ValidateLibraryRootName(name); err != nil {
			return AgentProfile{}, fmt.Errorf("profile %s: %w", path, err)
		}
		if strings.TrimSpace(directory) == "" {
			return AgentProfile{}, fmt.Errorf("profile %s: library root %q names no directory", path, name)
		}
	}
	return p, nil
}

func resolveProfilePaths(p *AgentProfile, base string) {
	for name, directory := range p.Libraries {
		p.Libraries[name] = resolveProfilePath(base, directory)
	}
	resolve := func(path string) string { return p.resolvePath(base, path) }
	p.Machine = resolve(p.Machine)
	for i, t := range p.Tools {
		p.Tools[i] = resolve(t)
	}
	for i, td := range p.ToolDeclarations {
		p.ToolDeclarations[i] = resolve(td)
	}
	for i, d := range p.ToolConfigDirs {
		p.ToolConfigDirs[i] = resolve(d)
	}
	for i, r := range p.RestDefinitions {
		p.RestDefinitions[i] = resolve(r)
	}
	for i, d := range p.RestConfigDirs {
		p.RestConfigDirs[i] = resolve(d)
	}
	if p.Directory != "" {
		p.Directory = resolve(p.Directory)
	}
}

// resolvePath resolves a profile-level path: an install-prefix path maps
// through the install root, a path under one of this profile's declared roots
// maps to that root's directory (srd056 R1.2), any other absolute path is used
// as written, and a relative path resolves against base.
func (p AgentProfile) resolvePath(base, path string) string {
	if resolved := resolveProfilePath(base, path); resolved != path || !filepath.IsAbs(path) {
		return resolved
	}
	for name, directory := range p.Libraries {
		prefix := corepath.LibraryPrefix + "/" + name
		clean := filepath.ToSlash(filepath.Clean(path))
		if strings.HasPrefix(clean, prefix+"/") {
			return filepath.Join(directory, filepath.FromSlash(strings.TrimPrefix(clean, prefix+"/")))
		}
	}
	return path
}

func resolveProfilePath(base, p string) string {
	if mapped := resolveInstalledAgentCorePath(p); mapped != "" {
		return mapped
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// ResolveConfiguredPath resolves an agent-core install path or a path relative
// to base. Nested profile-owned configuration uses the same mapping contract as
// top-level profile references.
func ResolveConfiguredPath(base, path string) string {
	return resolveProfilePath(base, path)
}

func resolveInstalledAgentCorePath(p string) string {
	return corepath.Map(p)
}
