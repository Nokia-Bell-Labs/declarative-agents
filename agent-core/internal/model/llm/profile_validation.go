// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type profilePrefix struct {
	value       string
	profileName string
	filename    string
}

type profileSpecYAML struct {
	ProfileName   string        `yaml:"name"`
	MatchPrefixes []string      `yaml:"match_prefixes"`
	MachineName   string        `yaml:"machine,omitempty"`
	Envelope      *EnvelopeSpec `yaml:"envelope"`
	StrictFormat  bool          `yaml:"strict_format"`
	Pipeline      []yaml.Node   `yaml:"extraction_pipeline"`
}

func (p *ProfileSpec) UnmarshalYAML(value *yaml.Node) error {
	var raw profileSpecYAML
	if err := value.Decode(&raw); err != nil {
		return err
	}

	steps := make([]PipelineStep, len(raw.Pipeline))
	for i := range raw.Pipeline {
		if err := raw.Pipeline[i].Decode(&steps[i]); err != nil {
			return fmt.Errorf("extraction_pipeline[%d]: %w", i, err)
		}
	}
	*p = ProfileSpec{
		ProfileName:   raw.ProfileName,
		MatchPrefixes: raw.MatchPrefixes,
		Envelope:      raw.Envelope,
		StrictFormat:  raw.StrictFormat,
		Pipeline:      steps,
	}
	if raw.MachineName != "" {
		return fmt.Errorf("response profile %q: machine is not supported; MachineSpec owns program selection", raw.ProfileName)
	}
	return nil
}

func validateProfileSpec(filename string, spec ProfileSpec) error {
	for i, prefix := range spec.MatchPrefixes {
		if strings.TrimSpace(prefix) == "" {
			return fmt.Errorf("profile %s: match_prefixes[%d] must not be empty", filename, i)
		}
	}
	for i, step := range spec.Pipeline {
		if err := validatePipelineStep(step); err != nil {
			return fmt.Errorf("profile %s: extraction_pipeline[%d]: %w", filename, i, err)
		}
	}
	return nil
}

func validatePipelineStep(step PipelineStep) error {
	switch step.Name {
	case "strip_code_fences", "strip_thinking_blocks", "extract_braces":
		return validateParameters(step, nil)
	case "extract_envelope":
		return validateParameters(step, []string{"open", "close"})
	case "extract_native_token":
		return validateParameters(step, []string{"token"})
	default:
		return fmt.Errorf("unknown operation %q", step.Name)
	}
}

func validateParameters(step PipelineStep, required []string) error {
	allowed := make(map[string]bool, len(required))
	for _, name := range required {
		allowed[name] = true
	}
	names := make([]string, 0, len(step.Params))
	for name := range step.Params {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !allowed[name] {
			return fmt.Errorf("operation %q does not allow parameter %q", step.Name, name)
		}
	}
	for _, name := range required {
		if strings.TrimSpace(step.Params[name]) == "" {
			return fmt.Errorf("operation %q requires non-empty parameter %q", step.Name, name)
		}
	}
	return nil
}

func validateProfileIdentity(reg *ProfileRegistry, filename string, spec ProfileSpec) error {
	if reg.profileFiles == nil {
		reg.profileFiles = make(map[string]string)
	}
	if previous, exists := reg.profileFiles[spec.ProfileName]; exists {
		return fmt.Errorf(
			"profile %s: duplicate name %q (already declared in %s)",
			filename, spec.ProfileName, previous,
		)
	}
	if isDefaultProfile(spec) && reg.defaultFile != "" {
		return fmt.Errorf("profile %s: duplicate default profile (already declared in %s)", filename, reg.defaultFile)
	}
	return validateProfilePrefixes(reg, filename, spec)
}

func validateProfilePrefixes(reg *ProfileRegistry, filename string, spec ProfileSpec) error {
	if isDefaultProfile(spec) {
		return nil
	}
	for _, prefix := range spec.MatchPrefixes {
		normalized := strings.ToLower(prefix)
		for _, previous := range reg.prefixSources {
			if strings.HasPrefix(normalized, previous.value) ||
				strings.HasPrefix(previous.value, normalized) {
				return fmt.Errorf(
					"profile %s: match prefix %q is ambiguous with %q from profile %q in %s",
					filename, prefix, previous.value, previous.profileName, previous.filename,
				)
			}
		}
	}
	return nil
}

func recordProfilePrefixes(reg *ProfileRegistry, filename string, spec ProfileSpec) {
	if isDefaultProfile(spec) {
		return
	}
	for _, prefix := range spec.MatchPrefixes {
		reg.prefixSources = append(reg.prefixSources, profilePrefix{
			value:       strings.ToLower(prefix),
			profileName: spec.ProfileName,
			filename:    filename,
		})
	}
}

func isDefaultProfile(spec ProfileSpec) bool {
	return spec.ProfileName == "default" || len(spec.MatchPrefixes) == 0
}
