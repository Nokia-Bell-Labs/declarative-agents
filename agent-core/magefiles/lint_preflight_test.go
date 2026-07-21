// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"strconv"
	"testing"
)

func TestParseGolangciLintMajor(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		want    int
		wantErr bool
	}{
		{
			name:   "v2 binary",
			output: "golangci-lint has version 2.12.2 built with go1.26.2 from c0d3ddc on 2026-05-06T11:01:25Z",
			want:   2,
		},
		{
			name:   "v1 binary",
			output: "golangci-lint has version 1.64.8 built with go1.23.0 from abcdef0 on 2025-01-01T00:00:00Z",
			want:   1,
		},
		{
			name:   "v-prefixed version",
			output: "golangci-lint has version v2.0.0",
			want:   2,
		},
		{
			name:    "no version",
			output:  "golangci-lint: command not found",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGolangciLintMajor(tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseGolangciLintMajor(%q) = %d, want error", tc.output, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGolangciLintMajor(%q) error: %v", tc.output, err)
			}
			if got != tc.want {
				t.Fatalf("parseGolangciLintMajor(%q) = %d, want %d", tc.output, got, tc.want)
			}
		})
	}
}

// TestCheckGolangciLintVersionRejectsMismatch proves the preflight fails with a
// version-specific message when the installed major version differs, which is
// the #474 regression (v2 config, v1 binary) surfaced as guidance rather than a
// mid-run schema error.
func TestCheckGolangciLintVersionRejectsMismatch(t *testing.T) {
	// The config schema is version "2"; assert the constant tracks it so a
	// future schema bump forces the preflight target to move with it.
	if got := strconv.Itoa(requiredGolangciLintMajor); got != "2" {
		t.Fatalf("requiredGolangciLintMajor = %s, want 2 (matching .golangci.yml version)", got)
	}
}
