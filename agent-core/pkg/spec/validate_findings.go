// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"sort"
	"strings"
)

type Finding struct {
	Check   string // short check identifier
	Level   string // "error" or "warning"
	Message string
	SuiteID string // optional charter suite identifier
	CheckID string // optional suite-local check identifier
	Kind    string // optional charter check kind
	File    string // optional target-relative file path
	Line    int    // optional 1-based line number
}

// Validate runs all consistency checks on the graph and returns findings.

func Errors(findings []Finding) []Finding {
	var errs []Finding
	for _, f := range findings {
		if f.Level == "error" {
			errs = append(errs, f)
		}
	}
	return errs
}

// Warnings returns only warning-level findings.

func Warnings(findings []Finding) []Finding {
	var warns []Finding
	for _, f := range findings {
		if f.Level == "warning" {
			warns = append(warns, f)
		}
	}
	return warns
}

// FormatFindings produces a sorted human-readable report.

func FormatFindings(findings []Finding) string {
	if len(findings) == 0 {
		return "All consistency checks passed.\n"
	}

	ordered := append([]Finding(nil), findings...)
	sort.Slice(ordered, func(i, j int) bool {
		return findingLess(ordered[i], ordered[j])
	})

	var b strings.Builder
	currentHeader := ""
	for _, f := range ordered {
		header := findingHeader(f)
		if header != currentHeader {
			currentHeader = header
			fmt.Fprintf(&b, "\n[%s] %s:\n", f.Level, header)
		}
		fmt.Fprintf(&b, "  - %s\n", findingLine(f))
	}
	return b.String()
}

func findingLess(a, b Finding) bool {
	if levelRank(a.Level) != levelRank(b.Level) {
		return levelRank(a.Level) < levelRank(b.Level)
	}
	for _, cmp := range []struct{ left, right string }{
		{a.SuiteID, b.SuiteID},
		{effectiveCheck(a), effectiveCheck(b)},
		{a.Kind, b.Kind},
		{a.File, b.File},
	} {
		if cmp.left != cmp.right {
			return cmp.left < cmp.right
		}
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Message < b.Message
}

func levelRank(level string) int {
	switch level {
	case "error":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func effectiveCheck(f Finding) string {
	if f.CheckID != "" {
		return f.CheckID
	}
	return f.Check
}

func findingHeader(f Finding) string {
	check := effectiveCheck(f)
	if check == "" {
		check = "unknown-check"
	}
	if f.SuiteID != "" {
		check = f.SuiteID + "/" + check
	}
	if f.Kind != "" {
		check += " (" + f.Kind + ")"
	}
	return check
}

func findingLine(f Finding) string {
	if f.File == "" {
		return f.Message
	}
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Message)
	}
	return fmt.Sprintf("%s: %s", f.File, f.Message)
}
