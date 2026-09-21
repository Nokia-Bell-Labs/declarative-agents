// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package chartconf

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Exception suspends one rule for one resource. Every field srd005 R7.1
// names is required: an entry that cannot say which issue removes it or why
// it stands is an exception nobody can retire.
type Exception struct {
	Chart    string `yaml:"chart"`
	Overlay  string `yaml:"overlay"`
	Rule     string `yaml:"rule"`
	Resource string `yaml:"resource"`
	Value    string `yaml:"value"`
	Issue    string `yaml:"issue"`
	Reason   string `yaml:"reason"`
}

func (e Exception) key() string {
	return strings.Join([]string{e.Chart, e.Overlay, e.Rule, e.Resource, e.Value}, "|")
}

// Baseline is the checked-in set of accepted violations.
type Baseline struct {
	Exceptions []Exception `yaml:"exceptions"`
}

// LoadBaseline parses a baseline document and rejects an entry missing any
// field R7.1 requires. An absent file is not this function's concern; the
// caller decides whether one is required.
func LoadBaseline(content string) (Baseline, error) {
	var baseline Baseline
	if err := yaml.Unmarshal([]byte(content), &baseline); err != nil {
		return Baseline{}, fmt.Errorf("parse chart conformance baseline: %w", err)
	}
	seen := map[string]bool{}
	for index, exception := range baseline.Exceptions {
		var missing []string
		for name, value := range map[string]string{
			"chart": exception.Chart, "rule": exception.Rule,
			"resource": exception.Resource, "issue": exception.Issue,
			"reason": exception.Reason,
		} {
			if strings.TrimSpace(value) == "" {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return Baseline{}, fmt.Errorf("baseline entry %d lacks %s",
				index, strings.Join(missing, ", "))
		}
		if seen[exception.key()] {
			return Baseline{}, fmt.Errorf("baseline entry %d duplicates an earlier entry: %s",
				index, exception.key())
		}
		seen[exception.key()] = true
	}
	return baseline, nil
}

// Reconcile compares findings against the baseline and returns the two ways
// the pair can be wrong: a violation nobody accepted, and an entry whose
// violation no longer reproduces (srd005 R7.2 and R7.3). Reconcile reads the
// baseline and never writes it.
func Reconcile(findings []Finding, baseline Baseline) (unaccepted []Finding, stale []Exception) {
	accepted := map[string]bool{}
	for _, exception := range baseline.Exceptions {
		accepted[exception.key()] = false
	}
	for _, finding := range findings {
		key := finding.Key()
		if _, listed := accepted[key]; listed {
			accepted[key] = true
			continue
		}
		unaccepted = append(unaccepted, finding)
	}
	for _, exception := range baseline.Exceptions {
		if !accepted[exception.key()] {
			stale = append(stale, exception)
		}
	}
	sort.Slice(unaccepted, func(i, j int) bool { return unaccepted[i].Key() < unaccepted[j].Key() })
	sort.Slice(stale, func(i, j int) bool { return stale[i].key() < stale[j].key() })
	return unaccepted, stale
}

// Report turns a reconciliation into one error naming everything that has to
// change, so a run reports every violation rather than the first.
func Report(unaccepted []Finding, stale []Exception) error {
	if len(unaccepted) == 0 && len(stale) == 0 {
		return nil
	}
	var lines []string
	for _, finding := range unaccepted {
		lines = append(lines, "  violation: "+finding.String())
	}
	for _, exception := range stale {
		lines = append(lines, fmt.Sprintf(
			"  stale baseline entry (%s no longer reproduces; remove it): %s [%s] %s %s %q",
			exception.Issue, exception.Chart, exception.Overlay, exception.Rule,
			exception.Resource, exception.Value))
	}
	return fmt.Errorf("chart conformance (ENG01, srd005):\n%s", strings.Join(lines, "\n"))
}
