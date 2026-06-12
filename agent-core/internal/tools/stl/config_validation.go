// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import "fmt"

// ValidateChildAgentConfig checks the declarative fields required to invoke a
// child agent through the agent CLI.
func ValidateChildAgentConfig(toolName string, cfg ChildAgentConfig) error {
	if cfg.Profile != "" {
		return nil
	}
	if cfg.Machine == "" {
		return fmt.Errorf("tool %q config requires profile or legacy machine", toolName)
	}
	if cfg.Tools == "" {
		return fmt.Errorf("tool %q config requires tools", toolName)
	}
	if len(cfg.ToolDeclarations) == 0 {
		return fmt.Errorf("tool %q config requires tools_declarations", toolName)
	}
	return nil
}

// ValidateRunPointConfig checks fields required to run a nested point machine.
func ValidateRunPointConfig(toolName string, cfg RunPointConfig) error {
	if cfg.PointMachine == "" {
		return fmt.Errorf("tool %q config requires point_machine", toolName)
	}
	if cfg.PointTools == "" {
		return fmt.Errorf("tool %q config requires point_tools", toolName)
	}
	return nil
}
