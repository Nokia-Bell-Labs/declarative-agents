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

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Level != findings[j].Level {
			return findings[i].Level == "error"
		}
		if findings[i].Check != findings[j].Check {
			return findings[i].Check < findings[j].Check
		}
		return findings[i].Message < findings[j].Message
	})

	var b strings.Builder
	currentCheck := ""
	for _, f := range findings {
		if f.Check != currentCheck {
			currentCheck = f.Check
			fmt.Fprintf(&b, "\n[%s] %s:\n", f.Level, f.Check)
		}
		fmt.Fprintf(&b, "  - %s\n", f.Message)
	}
	return b.String()
}
