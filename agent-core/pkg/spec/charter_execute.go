// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import "sort"

// ExecuteCharters runs loaded jurist charters over a target directory and spec corpus.
func ExecuteCharters(targetDir string, graph *Graph, corpus *Corpus, charters []Charter) ([]Finding, error) {
	if len(charters) == 0 {
		return Validate(graph, corpus), nil
	}

	var findings []Finding
	for _, charter := range charters {
		for _, check := range charter.Checks {
			checkCharter := Charter{
				ID:     charter.ID,
				Title:  charter.Title,
				Target: charter.Target,
				Checks: []CharterCheck{check},
				Path:   charter.Path,
			}
			checkFindings, err := executeCharterCheck(targetDir, graph, corpus, checkCharter, check)
			if err != nil {
				return nil, err
			}
			findings = append(findings, checkFindings...)
		}
	}
	sortFindings(findings)
	return findings, nil
}

func executeCharterCheck(targetDir string, graph *Graph, corpus *Corpus, charter Charter, check CharterCheck) ([]Finding, error) {
	switch check.Kind {
	case "spec_corpus":
		return executeSpecCorpusCheck(graph, corpus, charter, check), nil
	case "grep_check":
		return ExecuteGrepChecks(targetDir, []Charter{charter})
	case "ref_check":
		return ExecuteRefChecks(targetDir, []Charter{charter})
	case "consistency_check":
		return ExecuteConsistencyChecks(targetDir, []Charter{charter})
	default:
		return nil, nil
	}
}

func executeSpecCorpusCheck(graph *Graph, corpus *Corpus, charter Charter, check CharterCheck) []Finding {
	findings := Validate(graph, corpus)
	if len(check.Checks) > 0 {
		allowed := make(map[string]bool, len(check.Checks))
		for _, checkID := range check.Checks {
			allowed[checkID] = true
		}
		filtered := findings[:0]
		for _, finding := range findings {
			if allowed[finding.Check] {
				filtered = append(filtered, finding)
			}
		}
		findings = filtered
	}
	for i := range findings {
		if findings[i].SuiteID == "" {
			findings[i].SuiteID = charter.ID
		}
		if findings[i].CheckID == "" {
			findings[i].CheckID = findings[i].Check
		}
		if findings[i].Kind == "" {
			findings[i].Kind = check.Kind
		}
	}
	return findings
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		return findingLess(findings[i], findings[j])
	})
}
