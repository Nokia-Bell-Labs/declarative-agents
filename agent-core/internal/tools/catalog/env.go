// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	execEnvKeyPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	execEnvTokenPattern = regexp.MustCompile(`\{\{ params\.([A-Za-z_][A-Za-z0-9_]*) \}\}`)
)

func (td ToolDef) validateEnv() error {
	if !td.envSet && len(td.Env) == 0 {
		return nil
	}
	declared := td.declaredParamNames()
	for i, entry := range td.Env {
		key, value, err := SplitExecEnvEntry(entry)
		if err != nil {
			return fmt.Errorf("tool %q env[%d]: %w", td.Name, i, err)
		}
		names, err := ExecEnvTemplateNames(value)
		if err != nil {
			return fmt.Errorf("tool %q env %s: %w", td.Name, key, err)
		}
		for _, name := range names {
			if !declared[name] {
				return fmt.Errorf("tool %q env %s: {{ params.%s }} requires parameter %s", td.Name, key, name, name)
			}
		}
	}
	return nil
}

func (td ToolDef) declaredParamNames() map[string]bool {
	props, _ := td.Parameters["properties"].(map[string]interface{})
	names := make(map[string]bool, len(props))
	for name := range props {
		names[name] = true
	}
	return names
}

// SplitExecEnvEntry parses one KEY=VALUE env declaration.
func SplitExecEnvEntry(entry string) (key, value string, err error) {
	key, value, ok := strings.Cut(entry, "=")
	if !ok {
		return "", "", fmt.Errorf("must be KEY=VALUE")
	}
	if !execEnvKeyPattern.MatchString(key) {
		return "", "", fmt.Errorf("key %q must match [A-Za-z_][A-Za-z0-9_]*", key)
	}
	if value == "" {
		return "", "", fmt.Errorf("value must not be empty")
	}
	return key, value, nil
}

// ExecEnvTemplateNames returns params.NAME tokens in an env value.
func ExecEnvTemplateNames(value string) ([]string, error) {
	matches := execEnvTokenPattern.FindAllStringSubmatchIndex(value, -1)
	var names []string
	cursor := 0
	for _, loc := range matches {
		if strings.Contains(value[cursor:loc[0]], "{{") {
			return nil, fmt.Errorf("unknown env template token")
		}
		names = append(names, value[loc[2]:loc[3]])
		cursor = loc[1]
	}
	if strings.Contains(value[cursor:], "{{") {
		return nil, fmt.Errorf("unknown env template token")
	}
	return names, nil
}
