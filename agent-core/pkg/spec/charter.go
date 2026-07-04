// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var supportedCharterKinds = map[string]bool{
	"grep_check":        true,
	"ref_check":         true,
	"consistency_check": true,
	"spec_corpus":       true,
}

var supportedSpecCorpusCheckIDs = map[string]bool{
	"bare-touchpoint":                           true,
	"broken-citation":                           true,
	"broken-touchpoint":                         true,
	"depends-on-violation":                      true,
	"docspec-broken-example-path":               true,
	"docspec-broken-implementation-path":        true,
	"docspec-broken-related-document":           true,
	"docspec-broken-requirement-source":         true,
	"index-broken-path":                         true,
	"index-missing-test-suite":                  true,
	"index-missing-use-case":                    true,
	"machine-diagnostic-terminal_transition":    true,
	"machine-diagnostic-unreachable_state":      true,
	"machine-diagnostic-unreachable_transition": true,
	"machine-diagnostic-unused_signal":          true,
	"machine-incomplete-signal-metadata":        true,
	"machine-incomplete-state-metadata":         true,
	"machine-metric-label-invalid":              true,
	"machine-name-mismatch":                     true,
	"machine-unreceived-signal":                 true,
	"machine-unresolved-action":                 true,
	"orphaned-srd":                              true,
	"orphaned-test-suite":                       true,
	"release-without-test-suite":                true,
	"roadmap-missing-use-case":                  true,
	"test-case-missing-use-case":                true,
	"test-suite-missing-uc-trace":               true,
	"tool-boundary-category-missing":            true,
	"tool-contract-incomplete":                  true,
	"tool-emits-unknown-signal":                 true,
	"tool-metric-config-invalid":                true,
	"tool-selection-undeclared":                 true,
	"tool-undo-mismatch":                        true,
	"tool-undo-payload-no-captures":             true,
	"tool-unknown-side-effect-kind":             true,
	"uncovered-ac":                              true,
	"uncovered-req-item":                        true,
	"untraced-success-criterion":                true,
	"use-case-missing-test-suite":               true,
}

// Charter is a deterministic jurist check suite loaded from YAML.
type Charter struct {
	ID     string         `yaml:"id" json:"id"`
	Title  string         `yaml:"title,omitempty" json:"title,omitempty"`
	Target CharterTarget  `yaml:"target,omitempty" json:"target,omitempty"`
	Checks []CharterCheck `yaml:"checks" json:"checks"`
	Path   string         `yaml:"-" json:"path,omitempty"`
}

type CharterTarget struct {
	Root    string   `yaml:"root,omitempty" json:"root,omitempty"`
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

type CharterCheck struct {
	ID           string         `yaml:"id" json:"id"`
	Kind         string         `yaml:"kind" json:"kind"`
	Severity     string         `yaml:"severity,omitempty" json:"severity,omitempty"`
	Message      string         `yaml:"message,omitempty" json:"message,omitempty"`
	Include      []string       `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude      []string       `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Patterns     []string       `yaml:"patterns,omitempty" json:"patterns,omitempty"`
	Mode         string         `yaml:"mode,omitempty" json:"mode,omitempty"`
	Regex        bool           `yaml:"regex,omitempty" json:"regex,omitempty"`
	Refs         map[string]any `yaml:"references,omitempty" json:"references,omitempty"`
	Extract      map[string]any `yaml:"extract,omitempty" json:"extract,omitempty"`
	AllowMissing bool           `yaml:"allow_missing,omitempty" json:"allow_missing,omitempty"`
	Source       map[string]any `yaml:"source,omitempty" json:"source,omitempty"`
	Rule         string         `yaml:"rule,omitempty" json:"rule,omitempty"`
	Target       map[string]any `yaml:"target,omitempty" json:"target,omitempty"`
	Checks       []string       `yaml:"checks,omitempty" json:"checks,omitempty"`
}

// LoadCharters parses explicit jurist charter suite paths in deterministic order.
func LoadCharters(paths []string) ([]Charter, error) {
	paths = normalizedCharterPaths(paths)
	charters := make([]Charter, 0, len(paths))
	seenSuites := make(map[string]string, len(paths))
	for _, path := range paths {
		charter, err := LoadCharter(path)
		if err != nil {
			return nil, err
		}
		if prior, ok := seenSuites[charter.ID]; ok {
			return nil, fmt.Errorf("duplicate charter suite id %q in %s and %s", charter.ID, prior, path)
		}
		seenSuites[charter.ID] = path
		charters = append(charters, charter)
	}
	return charters, nil
}

// LoadCharter parses one jurist charter suite from disk.
func LoadCharter(path string) (Charter, error) {
	readPath := path
	if mapped := MapInstalledCorePath(path); mapped != "" {
		readPath = mapped
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		return Charter{}, fmt.Errorf("read charter %s: %w", path, err)
	}
	charter, err := ParseCharter(data)
	if err != nil {
		return Charter{}, fmt.Errorf("parse charter %s: %w", path, err)
	}
	charter.Path = path
	return charter, nil
}

// ParseCharter parses and validates one jurist charter suite.
func ParseCharter(data []byte) (Charter, error) {
	var charter Charter
	if err := yaml.Unmarshal(data, &charter); err != nil {
		return Charter{}, fmt.Errorf("invalid YAML: %w", err)
	}
	if err := validateCharter(&charter); err != nil {
		return Charter{}, err
	}
	return charter, nil
}

func normalizedCharterPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}

func validateCharter(charter *Charter) error {
	if charter.ID == "" {
		return fmt.Errorf("charter: missing id")
	}
	if len(charter.Checks) == 0 {
		return fmt.Errorf("charter %q: missing checks", charter.ID)
	}
	seenChecks := make(map[string]bool, len(charter.Checks))
	for i := range charter.Checks {
		check := &charter.Checks[i]
		if check.ID == "" {
			return fmt.Errorf("charter %q: check %d missing id", charter.ID, i+1)
		}
		if seenChecks[check.ID] {
			return fmt.Errorf("charter %q: duplicate check id %q", charter.ID, check.ID)
		}
		seenChecks[check.ID] = true
		if !supportedCharterKinds[check.Kind] {
			return fmt.Errorf("charter %q check %q: unknown check kind %q", charter.ID, check.ID, check.Kind)
		}
		if err := validateCharterCheckConfig(charter.ID, check); err != nil {
			return err
		}
		if check.Severity == "" {
			check.Severity = "error"
		}
		switch check.Severity {
		case "error", "warning":
		default:
			return fmt.Errorf("charter %q check %q: unknown severity %q", charter.ID, check.ID, check.Severity)
		}
	}
	return nil
}

func validateCharterCheckConfig(charterID string, check *CharterCheck) error {
	switch check.Kind {
	case "grep_check":
		if len(check.Patterns) == 0 {
			return fmt.Errorf("charter %q check %q: grep_check requires patterns", charterID, check.ID)
		}
		if check.Mode != "" && check.Mode != "match" && check.Mode != "missing" {
			return fmt.Errorf("charter %q check %q: unknown grep_check mode %q", charterID, check.ID, check.Mode)
		}
	case "ref_check":
		if len(check.Refs) == 0 {
			return fmt.Errorf("charter %q check %q: ref_check requires references", charterID, check.ID)
		}
		if raw, ok := stringMapValue(check.Extract, "regex"); !ok || raw == "" {
			return fmt.Errorf("charter %q check %q: ref_check requires extract.regex", charterID, check.ID)
		}
	case "consistency_check":
		if raw, ok := stringMapValue(check.Source, "yaml_path"); !ok || raw == "" {
			return fmt.Errorf("charter %q check %q: consistency_check requires source.yaml_path", charterID, check.ID)
		}
		if check.Rule == "" {
			return fmt.Errorf("charter %q check %q: consistency_check requires rule", charterID, check.ID)
		}
		switch check.Rule {
		case "equals", "required_path_exists", "required_when":
		default:
			return fmt.Errorf("charter %q check %q: unknown consistency_check rule %q", charterID, check.ID, check.Rule)
		}
	case "spec_corpus":
		if err := validateSpecCorpusSubset(charterID, check); err != nil {
			return err
		}
	}
	return nil
}

func validateSpecCorpusSubset(charterID string, check *CharterCheck) error {
	for _, checkID := range check.Checks {
		if !supportedSpecCorpusCheckIDs[checkID] {
			return fmt.Errorf("charter %q check %q: unknown spec_corpus check %q", charterID, check.ID, checkID)
		}
	}
	return nil
}
