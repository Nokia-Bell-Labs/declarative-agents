// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package plan

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	agentllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
)

// ParsePlan extracts an ImplementationPlan from a raw LLM response.
// The response may contain YAML wrapped in markdown code fences or
// bare YAML. Returns a descriptive error for empty input, unparseable
// YAML, or missing required fields.
func ParsePlan(raw string) (ImplementationPlan, error) {
	if strings.TrimSpace(raw) == "" {
		return ImplementationPlan{}, fmt.Errorf("parse plan: empty input")
	}

	cleaned := agentllm.StripCodeFences(raw)

	var p ImplementationPlan
	if err := yaml.Unmarshal([]byte(cleaned), &p); err != nil {
		return ImplementationPlan{}, fmt.Errorf("parse plan: invalid YAML: %w", err)
	}

	if err := validatePlan(p); err != nil {
		return ImplementationPlan{}, err
	}

	return p, nil
}

// validatePlan checks that required fields are populated.
func validatePlan(p ImplementationPlan) error {
	if p.Title == "" {
		return fmt.Errorf("parse plan: missing required field: title")
	}
	if len(p.Requirements) == 0 {
		return fmt.Errorf("parse plan: missing required field: requirements (list is empty)")
	}
	if len(p.AcceptanceCriteria) == 0 {
		return fmt.Errorf("parse plan: missing required field: acceptance_criteria (list is empty)")
	}
	return nil
}
