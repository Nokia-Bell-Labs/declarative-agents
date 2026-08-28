// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func (c *ExecCmd) resolveEnv() ([]string, error) {
	if len(c.def.Env) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(c.def.Env))
	for i, entry := range c.def.Env {
		key, raw, err := catalog.SplitExecEnvEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("env[%d]: %w", i, err)
		}
		value, err := expandEnvValue(raw, c.params)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", key, err)
		}
		out = append(out, key+"="+value)
	}
	return out, nil
}

func expandEnvValue(raw string, params map[string]string) (string, error) {
	names, err := catalog.ExecEnvTemplateNames(raw)
	if err != nil {
		return "", err
	}
	expanded := raw
	for _, name := range names {
		value, ok := params[name]
		if !ok {
			return "", fmt.Errorf("parameter %q is missing", name)
		}
		if value == "" {
			return "", fmt.Errorf("parameter %q resolved to an empty string", name)
		}
		expanded = strings.ReplaceAll(expanded, "{{ params."+name+" }}", value)
	}
	if expanded == "" {
		return "", fmt.Errorf("value expanded to an empty string")
	}
	return expanded, nil
}
