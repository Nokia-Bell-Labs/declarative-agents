// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"
)

func TestParseGolangciLintMajor(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    int
		wantErr bool
	}{
		{
			name:   "v2 release output",
			output: "golangci-lint has version 2.12.2 built with go1.26.5 from (unknown) on (unknown)",
			want:   2,
		},
		{
			name:   "v1 release output",
			output: "golangci-lint has version 1.64.8 built with go1.26.3 from abcdef0 on 2025-01-01T00:00:00Z",
			want:   1,
		},
		{
			name:   "v prefix",
			output: "golangci-lint has version v2.0.0",
			want:   2,
		},
		{
			name:    "not installed",
			output:  "golangci-lint: command not found",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			major, err := parseGolangciLintMajor(tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseGolangciLintMajor(%q) = %d, want an error", tc.output, major)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if major != tc.want {
				t.Errorf("major = %d, want %d", major, tc.want)
			}
		})
	}
}

// TestRequiredGolangciLintMajorComesFromTheConfigs pins the single source of
// truth: the requirement is whatever schema the module configs declare, so it
// cannot drift from them the way a constant could.
func TestRequiredGolangciLintMajorComesFromTheConfigs(t *testing.T) {
	required, err := requiredGolangciLintMajor()
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range lintModuleDirs {
		declared, err := golangciConfigSchema(module)
		if err != nil {
			t.Fatalf("%s: %v", module, err)
		}
		if declared != required {
			t.Errorf("%s declares schema v%d, required = v%d", module, declared, required)
		}
	}
	if required < 2 {
		t.Errorf("required schema = v%d, want the v2 schema the configs are written against", required)
	}
}

// TestGolangciConfigSchemaReportsAMissingConfig keeps the preflight's failure
// legible when a module is added without a config, rather than defaulting to some
// version and linting under an unintended policy.
func TestGolangciConfigSchemaReportsAMissingConfig(t *testing.T) {
	_, err := golangciConfigSchema("applications/does-not-exist")
	if err == nil {
		t.Fatal("a missing lint config was accepted")
	}
	if !strings.Contains(err.Error(), "read lint config") {
		t.Errorf("error = %v, want it to name the unreadable config", err)
	}
}

// TestCheckGolangciLintAcceptsTheInstalledBinary is the end-to-end shape of the
// preflight. It skips rather than fails where no binary is installed, because
// that is the contributor state the guidance exists for, and asserting it would
// make the suite depend on a tool the suite does not need.
func TestCheckGolangciLintAcceptsTheInstalledBinary(t *testing.T) {
	major, err := installedGolangciLintMajor()
	if err != nil {
		t.Skipf("golangci-lint not usable here: %v", err)
	}
	required, err := requiredGolangciLintMajor()
	if err != nil {
		t.Fatal(err)
	}
	checkErr := checkGolangciLint()
	if major == required && checkErr != nil {
		t.Fatalf("matching binary v%d was rejected: %v", major, checkErr)
	}
	if major != required {
		if checkErr == nil {
			t.Fatalf("mismatched binary v%d (want v%d) was accepted", major, required)
		}
		for _, want := range []string{"is required by .golangci.yml", golangciLintInstallURL} {
			if !strings.Contains(checkErr.Error(), want) {
				t.Errorf("guidance %q does not mention %q", checkErr.Error(), want)
			}
		}
	}
}
