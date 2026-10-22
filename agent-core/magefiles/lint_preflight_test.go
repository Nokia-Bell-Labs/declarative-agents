// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseGolangciLintVersion(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{
			name:   "v2 binary",
			output: "golangci-lint has version 2.15.0 built with go1.27.2 from 25c5369 on 2026-10-16T10:51:54Z",
			want:   "2.15.0",
		},
		{
			name:   "v1 binary",
			output: "golangci-lint has version 1.64.8 built with go1.23.0 from abcdef0 on 2025-01-01T00:00:00Z",
			want:   "1.64.8",
		},
		{
			name:   "v-prefixed version",
			output: "golangci-lint has version v2.0.0",
			want:   "2.0.0",
		},
		{
			name:    "no version",
			output:  "golangci-lint: command not found",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGolangciLintVersion(tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseGolangciLintVersion(%q) = %q, want error", tc.output, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGolangciLintVersion(%q) error: %v", tc.output, err)
			}
			if got != tc.want {
				t.Fatalf("parseGolangciLintVersion(%q) = %q, want %q", tc.output, got, tc.want)
			}
		})
	}
}

// TestCheckPinnedGolangciLintRejectsAnyOtherRelease proves the preflight
// refuses a different major release (the #474 regression: v2 config, v1
// binary) and a different minor release (GH-2707: 2.15 errcheck findings on
// unchanged code) with guidance naming the pin.
func TestCheckPinnedGolangciLintRejectsAnyOtherRelease(t *testing.T) {
	if err := checkPinnedGolangciLint("2.15.0", "v2.15.0"); err != nil {
		t.Fatalf("the pinned release was refused: %v", err)
	}
	for _, installed := range []string{"1.64.8", "2.12.2", "2.16.0"} {
		err := checkPinnedGolangciLint(installed, "v2.15.0")
		if err == nil {
			t.Fatalf("golangci-lint %s was accepted against the v2.15.0 pin", installed)
		}
		if !strings.Contains(err.Error(), "v2.15.0 is pinned") || !strings.Contains(err.Error(), golangciLintInstallURL) {
			t.Errorf("guidance %q does not name the pin and where to install it", err.Error())
		}
	}
}

// TestPinnedGolangciLintMatchesTheConfigSchema keeps the pin a release that can
// read .golangci.yml at all, so a schema bump forces the pin to move with it.
func TestPinnedGolangciLintMatchesTheConfigSchema(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if want := "v" + strings.TrimSpace(config.Version) + "."; !strings.HasPrefix(requiredGolangciLintVersion, want) {
		t.Fatalf("requiredGolangciLintVersion = %s, want a release for .golangci.yml schema %q", requiredGolangciLintVersion, config.Version)
	}
}
