// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ContractBaselineFile is the tracked ratchet for tool-contract completeness.
// It lists the declared words whose six-section contract is still incomplete.
// The list may shrink and must never grow (GH-1525).
const ContractBaselineFile = "pkg/spec/testdata/tool-contract-completeness-baseline.yaml"

type contractBaseline struct {
	Incomplete []contractBaselineEntry `yaml:"incomplete"`
}

type contractBaselineEntry struct {
	Tool    string   `yaml:"tool"`
	Source  string   `yaml:"source"`
	Missing []string `yaml:"missing"`
}

// checkDeclaredToolContractCompleteness reports contract completeness over every
// declared word, not only the words a machine selects.
//
// Selection-scoped checking is why the gap in GH-1525 went unnoticed: a corpus
// with no machines selects nothing, so the selected-tool check iterated an empty
// set and passed. This check reads the declarations directly and gates them
// against the recorded baseline: a word missing from the baseline is an error, a
// baseline entry that is now complete is an error telling the author to ratchet
// the baseline down, and a baseline entry that is still incomplete is a warning.
func checkDeclaredToolContractCompleteness(corpus *Corpus) []Finding {
	if corpus == nil || len(corpus.ToolDeclarations) == 0 {
		return nil
	}
	baseline, ok, err := loadContractBaseline(corpus.RootDir)
	if err != nil {
		return []Finding{contractError("tool-contract-baseline-unreadable", err.Error())}
	}
	if !ok {
		// A corpus that ships no baseline is not gated. Only agent-core, which
		// owns the shared vocabulary, carries one.
		return nil
	}

	names := make([]string, 0, len(corpus.ToolDeclarations))
	for name := range corpus.ToolDeclarations {
		names = append(names, name)
	}
	sort.Strings(names)

	var findings []Finding
	for _, name := range names {
		expected, listed := baseline[name]
		if finding, produced := compareContractToBaseline(
			name, corpus.ToolDeclarations[name], expected, listed,
		); produced {
			findings = append(findings, finding)
		}
	}
	return append(findings, staleBaselineEntries(corpus, baseline)...)
}

// staleBaselineEntries reports baseline entries for words the corpus no longer
// declares, so a removed word cannot leave the ratchet loose.
func staleBaselineEntries(corpus *Corpus, baseline map[string][]string) []Finding {
	var findings []Finding
	for _, name := range sortedBaselineNames(baseline) {
		if _, declared := corpus.ToolDeclarations[name]; declared {
			continue
		}
		findings = append(findings, contractError("tool-contract-baseline-stale", fmt.Sprintf(
			"%s lists tool %q, which is no longer declared; remove it",
			ContractBaselineFile, name)))
	}
	return findings
}

// compareContractToBaseline classifies one declared word against its baseline
// entry. The second return reports whether a finding was produced; a complete
// and unlisted word is the desired state and produces none.
func compareContractToBaseline(name string, td ToolDeclaration, expected []string, listed bool) (Finding, bool) {
	missing := missingToolContractFields(td)
	source := sourceOrUnknown(td.SourceFile)

	switch {
	case len(missing) == 0 && !listed:
		return Finding{}, false
	case len(missing) == 0:
		return contractError("tool-contract-baseline-stale", fmt.Sprintf(
			"tool %q now has a complete contract; remove it from %s",
			name, ContractBaselineFile)), true
	case !listed:
		return contractError("tool-contract-incomplete-new", fmt.Sprintf(
			"tool %q from %s is missing contract fields: %s. Complete the contract; do not add it to %s",
			name, source, strings.Join(missing, ", "), ContractBaselineFile)), true
	case !sameFields(missing, expected):
		return contractError("tool-contract-baseline-drift", fmt.Sprintf(
			"tool %q missing fields changed from [%s] to [%s]; update %s",
			name, strings.Join(expected, ", "), strings.Join(missing, ", "), ContractBaselineFile)), true
	default:
		return Finding{
			Check: "tool-contract-incomplete",
			Level: "warning",
			Message: fmt.Sprintf(
				"tool %q from %s is missing contract fields: %s (recorded in %s)",
				name, source, strings.Join(missing, ", "), ContractBaselineFile),
		}, true
	}
}

func contractError(check, message string) Finding {
	return Finding{Check: check, Level: "error", Message: message}
}

// checkUnresolvedDeclarationFiles reports declaration paths a profile named that
// could not be read. A profile that points at an absolute container path is
// normal on a host checkout, so this warns rather than failing the audit.
func checkUnresolvedDeclarationFiles(corpus *Corpus) []Finding {
	if corpus == nil {
		return nil
	}
	findings := make([]Finding, 0, len(corpus.UnresolvedDeclFiles))
	for _, path := range corpus.UnresolvedDeclFiles {
		findings = append(findings, Finding{
			Check: "tool-declaration-path-unresolved",
			Level: "warning",
			Message: fmt.Sprintf(
				"declared tool declaration file %s could not be read; its words are absent from the corpus",
				path,
			),
		})
	}
	return findings
}

func loadContractBaseline(rootDir string) (map[string][]string, bool, error) {
	path := filepath.Join(rootDir, ContractBaselineFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", ContractBaselineFile, err)
	}
	var file contractBaseline
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", ContractBaselineFile, err)
	}
	baseline := make(map[string][]string, len(file.Incomplete))
	for _, entry := range file.Incomplete {
		if entry.Tool == "" {
			return nil, false, fmt.Errorf("%s: entry with no tool name", ContractBaselineFile)
		}
		fields := append([]string(nil), entry.Missing...)
		sort.Strings(fields)
		baseline[entry.Tool] = fields
	}
	return baseline, true, nil
}

func sortedBaselineNames(baseline map[string][]string) []string {
	names := make([]string, 0, len(baseline))
	for name := range baseline {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sameFields(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	got := append([]string(nil), actual...)
	sort.Strings(got)
	for i := range got {
		if got[i] != expected[i] {
			return false
		}
	}
	return true
}

// ContractGap names one declared word whose contract is incomplete and the
// sections it is missing.
type ContractGap struct {
	Tool    string
	Source  string
	Missing []string
}

// IncompleteToolContracts reports every declared word whose six-section
// contract is incomplete, in name order. It is the same rule the audit gates
// on, exposed so the baseline file can be regenerated from it rather than
// maintained by hand.
func IncompleteToolContracts(corpus *Corpus) []ContractGap {
	if corpus == nil {
		return nil
	}
	names := make([]string, 0, len(corpus.ToolDeclarations))
	for name := range corpus.ToolDeclarations {
		names = append(names, name)
	}
	sort.Strings(names)

	var gaps []ContractGap
	for _, name := range names {
		td := corpus.ToolDeclarations[name]
		missing := missingToolContractFields(td)
		if len(missing) == 0 {
			continue
		}
		sort.Strings(missing)
		gaps = append(gaps, ContractGap{Tool: name, Source: td.SourceFile, Missing: missing})
	}
	return gaps
}
