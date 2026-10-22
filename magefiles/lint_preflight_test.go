// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestParseGolangciLintVersion(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{
			name:   "v2 release output",
			output: "golangci-lint has version 2.15.0 built with go1.27.2 from 25c5369 on 2026-10-16T10:51:54Z",
			want:   "2.15.0",
		},
		{
			name:   "v1 release output",
			output: "golangci-lint has version 1.64.8 built with go1.26.3 from abcdef0 on 2025-01-01T00:00:00Z",
			want:   "1.64.8",
		},
		{
			name:   "v prefix",
			output: "golangci-lint has version v2.0.0",
			want:   "2.0.0",
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
			got, err := parseGolangciLintVersion(tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseGolangciLintVersion(%q) = %q, want an error", tc.output, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("version = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCheckPinnedGolangciLint refuses any release but the pinned one, a minor
// upgrade included, and says what to install (GH-2710 R1).
func TestCheckPinnedGolangciLint(t *testing.T) {
	if err := checkPinnedGolangciLint("2.15.0", "v2.15.0"); err != nil {
		t.Fatalf("the pinned release was refused: %v", err)
	}
	for _, installed := range []string{"2.16.0", "2.12.2", "1.64.8"} {
		err := checkPinnedGolangciLint(installed, "v2.15.0")
		if err == nil {
			t.Fatalf("golangci-lint %s was accepted against the v2.15.0 pin", installed)
		}
		for _, want := range []string{"v2.15.0 is pinned", "v" + installed + " is installed", golangciLintInstallURL} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("guidance %q does not mention %q", err.Error(), want)
			}
		}
	}
}

// TestPinnedGolangciLintMatchesTheConfigSchema keeps the pin a release that can
// read the configs at all.
func TestPinnedGolangciLintMatchesTheConfigSchema(t *testing.T) {
	required, err := requiredGolangciLintMajor()
	if err != nil {
		t.Fatal(err)
	}
	if want := "v" + strconv.Itoa(required) + "."; !strings.HasPrefix(golangciLintVersion, want) {
		t.Errorf("golangciLintVersion = %s, want a v%d release for the configs' schema", golangciLintVersion, required)
	}
}

// TestAgentCorePinsTheSameGolangciLint holds agent-core's standalone Lint to
// the root's pin; its module cannot import this constant (GH-2710 R2).
func TestAgentCorePinsTheSameGolangciLint(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "agent-core", "magefiles", "build.go"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`const requiredGolangciLintVersion = "([^"]+)"`).FindSubmatch(data)
	if match == nil {
		t.Fatal("agent-core/magefiles/build.go declares no requiredGolangciLintVersion")
	}
	if string(match[1]) != golangciLintVersion {
		t.Errorf("agent-core pins golangci-lint %s, the root pins %s", match[1], golangciLintVersion)
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
	installed, err := installedGolangciLintVersion()
	if err != nil {
		t.Skipf("golangci-lint not usable here: %v", err)
	}
	checkErr := checkGolangciLint()
	pinned := strings.TrimPrefix(golangciLintVersion, "v") == installed
	if pinned && checkErr != nil {
		t.Fatalf("the pinned release %s was rejected: %v", installed, checkErr)
	}
	if !pinned && checkErr == nil {
		t.Fatalf("golangci-lint %s was accepted against the %s pin", installed, golangciLintVersion)
	}
}
