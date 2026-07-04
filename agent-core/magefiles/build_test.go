// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditRunFailed(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "terminal state failed",
			output: `2026/06/16 20:57:05 run complete: status=failed iterations=1
terminal state: failed
`,
			want: true,
		},
		{
			name:   "run summary failed",
			output: "2026/06/16 20:57:05 run complete: status=failed iterations=1\n",
			want:   true,
		},
		{
			name:   "succeeded",
			output: "2026/06/16 20:57:05 run complete: status=succeeded iterations=1\nterminal state: succeeded\n",
			want:   false,
		},
		{
			name:   "empty",
			output: "",
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := auditRunFailed(tc.output)
			if got != tc.want {
				t.Fatalf("auditRunFailed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnvWithDefault(t *testing.T) {
	t.Parallel()

	got := envWithDefault([]string{"PATH=/bin"}, "TEST_OVERRIDE_PATH", "/repo")
	if !containsEnv(got, "TEST_OVERRIDE_PATH=/repo") {
		t.Fatalf("envWithDefault() = %v, want TEST_OVERRIDE_PATH default", got)
	}

	existing := []string{"TEST_OVERRIDE_PATH=/custom"}
	got = envWithDefault(existing, "TEST_OVERRIDE_PATH", "/repo")
	if len(got) != 1 || got[0] != "TEST_OVERRIDE_PATH=/custom" {
		t.Fatalf("envWithDefault() = %v, want existing value preserved", got)
	}
}

func TestWriteJuristCharterSmokeProfile(t *testing.T) {
	coreRoot := t.TempDir()
	profilesRepoRoot := t.TempDir()
	mkdirAll(t, filepath.Join(profilesRepoRoot, "agents", "jurist", "suites"))

	profilePath, cleanup, err := writeJuristCharterSmokeProfile(coreRoot, profilesRepoRoot)
	if err != nil {
		t.Fatalf("writeJuristCharterSmokeProfile returned error: %v", err)
	}
	defer cleanup()

	profileData, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	toolDeclData, err := os.ReadFile(filepath.Join(filepath.Dir(profilePath), "load-corpus-demo.yaml"))
	if err != nil {
		t.Fatalf("read tool declaration: %v", err)
	}
	profile := string(profileData)
	toolDecl := string(toolDeclData)
	if !strings.Contains(profile, filepath.Join(profilesRepoRoot, "agents", "jurist", "machine.yaml")) {
		t.Fatalf("profile = %q, want jurist machine path", profile)
	}
	if !strings.Contains(profile, filepath.Join(coreRoot, "tools", "builtin", "spec-validation")) {
		t.Fatalf("profile = %q, want core spec-validation dir", profile)
	}
	if !strings.Contains(toolDecl, filepath.Join(coreRoot, "tools", "builtin", "load-corpus.yaml")) {
		t.Fatalf("tool declaration = %q, want core load_corpus include", toolDecl)
	}
	if !strings.Contains(toolDecl, filepath.Join(profilesRepoRoot, "agents", "jurist", "suites", "demo-charter.yaml")) {
		t.Fatalf("tool declaration = %q, want demo charter suite path", toolDecl)
	}
}

func TestAssertJuristCharterSmoke(t *testing.T) {
	output := `
[error] jurist-demo-charter/no-internal-vocabulary (grep_check) at docs/manuscript.md:3
[error] jurist-demo-charter/citations-resolve (ref_check) at docs/manuscript.md:5
[error] jurist-demo-charter/artifacts-exist (consistency_check) at manifest.yaml:2
terminal state: failed
`
	if err := assertJuristCharterSmoke(output); err != nil {
		t.Fatalf("assertJuristCharterSmoke returned error: %v", err)
	}
}

func TestAssertJuristCharterSmokeReportsMissingFinding(t *testing.T) {
	err := assertJuristCharterSmoke("terminal state: failed")
	if err == nil {
		t.Fatal("assertJuristCharterSmoke returned nil error for missing findings")
	}
	if !strings.Contains(err.Error(), "grep_check") {
		t.Fatalf("error = %q, want missing grep_check", err)
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
