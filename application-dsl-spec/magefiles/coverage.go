// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// coverageFileName is the map from the engine's format references onto the
// statement database, kept beside the statements it maps.
const coverageFileName = "COVERAGE.md"

// coverageVerdicts are the verdicts a coverage row may take. covered names
// the statements that state the rule; detail marks behaviour the language
// deliberately does not state, with its kind; gap names an area no statement
// yet states and the issue that tracks it.
var coverageVerdicts = map[string]bool{"covered": true, "detail": true, "gap": true}

// coverageDetailKinds are the kinds a detail row must name, so a reviewer can
// challenge the classification rather than read an excuse.
var coverageDetailKinds = []string{
	"presentation", "observability", "engine policy", "deployment",
	"repository layout", "tooling", "authoring guidance",
}

var issueReferencePattern = regexp.MustCompile(`#[0-9]+\b`)

// coverageRow is one area of one format reference.
type coverageRow struct {
	Line    int
	Area    string
	Verdict string
	Where   string
}

// checkCoverageMap holds the version's coverage map to the statement
// database: every row takes a known verdict; a covered row names statements
// that exist; a detail row names its kind; a gap row names a tracking issue.
func checkCoverageMap(versionRoot string, language languageFile) error {
	path := filepath.Join(versionRoot, coverageFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read coverage map: %w", err)
	}
	rows, err := parseCoverageRows(string(data))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%s holds no coverage rows", coverageFileName)
	}
	ids := make(map[string]bool, len(language.Statements))
	for _, statement := range language.Statements {
		ids[statement.ID] = true
	}

	var findings []error
	for _, row := range rows {
		label := fmt.Sprintf("%s:%d (%s)", coverageFileName, row.Line, row.Area)
		switch row.Verdict {
		case "covered":
			cited := citationIDPattern.FindAllString(row.Where, -1)
			if len(cited) == 0 {
				findings = append(findings, fmt.Errorf("%s: covered row names no statement", label))
			}
			for _, id := range cited {
				if !ids[id] {
					findings = append(findings, fmt.Errorf("%s: %s resolves to no statement", label, id))
				}
			}
		case "detail":
			if !namesDetailKind(row.Where) {
				findings = append(findings, fmt.Errorf("%s: detail row must name its kind (%s)",
					label, strings.Join(coverageDetailKinds, ", ")))
			}
		case "gap":
			if !issueReferencePattern.MatchString(row.Where) {
				findings = append(findings, fmt.Errorf("%s: gap row must name a tracking issue", label))
			}
		default:
			findings = append(findings, fmt.Errorf("%s: unknown verdict %q", label, row.Verdict))
		}
	}
	if len(findings) > 0 {
		return errors.Join(findings...)
	}
	fmt.Printf("validated %d coverage rows\n", len(rows))
	return nil
}

func namesDetailKind(where string) bool {
	lower := strings.ToLower(where)
	for _, kind := range coverageDetailKinds {
		if strings.Contains(lower, kind) {
			return true
		}
	}
	return false
}

// parseCoverageRows reads every body row of every `| Area | Verdict | Where |`
// table in the document.
func parseCoverageRows(document string) ([]coverageRow, error) {
	var rows []coverageRow
	inTable := false
	for index, line := range strings.Split(document, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			inTable = false
			continue
		}
		cells := splitTableRow(trimmed)
		if len(cells) == 3 && cells[0] == "Area" && cells[1] == "Verdict" && cells[2] == "Where" {
			inTable = true
			continue
		}
		if !inTable || isSeparatorRow(cells) {
			continue
		}
		if len(cells) != 3 {
			return nil, fmt.Errorf("%s:%d: coverage row has %d cells, want 3", coverageFileName, index+1, len(cells))
		}
		rows = append(rows, coverageRow{Line: index + 1, Area: cells[0], Verdict: cells[1], Where: cells[2]})
	}
	return rows, nil
}

func splitTableRow(line string) []string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells
}

func isSeparatorRow(cells []string) bool {
	for _, cell := range cells {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return true
}
