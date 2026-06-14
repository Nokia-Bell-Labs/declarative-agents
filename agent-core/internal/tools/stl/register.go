// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"embed"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

//go:embed tools.yaml
var defaultToolsFS embed.FS

// DefaultToolDefs is a compatibility wrapper for loading the legacy embedded
// exec declarations. New code should load declarations through catalog.
func DefaultToolDefs() ([]ToolDef, error) {
	data, err := defaultToolsFS.ReadFile("tools.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded tools.yaml: %w", err)
	}
	return ParseToolDefs(data)
}

// RegisterFileTools is a compatibility wrapper for filesystem tool builders.
// New code should import internal/tools/filesystem directly.
func RegisterFileTools(reg *core.Registry, root string) {
	reg.Register(ReadToolSpec(), &ReadBuilder{Root: root})
	reg.Register(WriteToolSpec(), &WriteBuilder{Root: root})
	reg.Register(EditToolSpec(), &EditBuilder{Root: root})
	reg.Register(FindToolSpec(), &FindBuilder{Root: root})
	reg.Register(ListFilesToolSpec(), &ListFilesBuilder{Root: root})
}

// RegisterExecTools is a compatibility wrapper for legacy YAML-defined exec
// tools. New code should use catalog declarations and registry wiring.
func RegisterExecTools(reg *core.Registry, root string) error {
	defs, err := DefaultToolDefs()
	if err != nil {
		return err
	}
	RegisterToolDefs(reg, root, defs)
	return nil
}

// RegisterExecToolsFrom is a compatibility wrapper for legacy exec overrides.
// New code should load declarations through catalog and register via registry.
func RegisterExecToolsFrom(reg *core.Registry, root, yamlPath string) error {
	defaults, err := DefaultToolDefs()
	if err != nil {
		return err
	}
	overrides, err := LoadToolDefs(yamlPath)
	if err != nil {
		return err
	}
	merged := MergeToolDefs(defaults, overrides)
	RegisterToolDefs(reg, root, merged)
	return nil
}

// RegisterAll is a compatibility wrapper for legacy standard tool registration.
// New code should use focused tool packages and registry.RegisterUnifiedTools.
func RegisterAll(reg *core.Registry, root string) error {
	RegisterFileTools(reg, root)
	return RegisterExecTools(reg, root)
}
