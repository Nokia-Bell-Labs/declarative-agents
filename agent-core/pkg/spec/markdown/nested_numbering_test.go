// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"strings"
	"testing"
)

// A nested list numbered 1., 2. is refused for its number, not its
// indentation: every item of a nested list is written "1.".
func TestTreeNamesAMisnumberedItem(t *testing.T) {
	_, err := parseTree("- depends_on:\n  1. ep001-cloud-foundation\n  2. ep002-agent-runtime\n")
	if err == nil || !strings.Contains(err.Error(), `is numbered 2.; number every item of a nested list "1."`) {
		t.Fatalf("err = %v", err)
	}
	if _, err := parseTree("- depends_on:\n  1. ep001-cloud-foundation\n  1. ep002-agent-runtime\n"); err != nil {
		t.Fatalf("a list numbered 1., 1. does not parse: %v", err)
	}
}
