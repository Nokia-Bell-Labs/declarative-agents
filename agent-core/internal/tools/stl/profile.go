// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AgentProfile bundles all configuration an agent needs into a single
// file. All paths are resolved relative to the profile file's directory.
type AgentProfile struct {
	Name             string   `yaml:"name"`
	Machine          string   `yaml:"machine"`
	Tools            []string `yaml:"tools"`
	ToolDeclarations []string `yaml:"tool_declarations"`
	ToolConfigDirs   []string `yaml:"tool_config_dirs,omitempty"`
	Directory        string   `yaml:"directory,omitempty"`
}

// LoadProfile reads a profile YAML file and resolves all relative paths
// against the profile file's directory.
func LoadProfile(path string) (AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentProfile{}, fmt.Errorf("load profile %s: %w", path, err)
	}
	var p AgentProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return AgentProfile{}, fmt.Errorf("parse profile %s: %w", path, err)
	}

	if p.Machine == "" {
		return AgentProfile{}, fmt.Errorf("profile %s: machine is required", path)
	}
	if len(p.Tools) == 0 {
		return AgentProfile{}, fmt.Errorf("profile %s: at least one tools entry is required", path)
	}

	base := filepath.Dir(path)
	p.Machine = resolveProfilePath(base, p.Machine)
	for i, t := range p.Tools {
		p.Tools[i] = resolveProfilePath(base, t)
	}
	for i, td := range p.ToolDeclarations {
		p.ToolDeclarations[i] = resolveProfilePath(base, td)
	}
	for i, d := range p.ToolConfigDirs {
		p.ToolConfigDirs[i] = resolveProfilePath(base, d)
	}
	if p.Directory != "" {
		p.Directory = resolveProfilePath(base, p.Directory)
	}
	return p, nil
}

func resolveProfilePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}
