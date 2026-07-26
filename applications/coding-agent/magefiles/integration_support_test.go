// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog/catalogroot"
)

func TestCodingAgentCatalogRootEnvironmentPrecedence(t *testing.T) {
	startup := t.TempDir()
	canonical := codingAgentCatalogFixture(t, filepath.Join(startup, "canonical"))
	legacy := codingAgentCatalogFixture(t, filepath.Join(startup, "legacy"))

	tests := []struct {
		name      string
		canonical string
		legacy    string
		want      string
		wantError []string
	}{
		{name: "canonical only", canonical: canonical, want: canonical},
		{name: "legacy only", legacy: legacy, want: legacy},
		{
			name:      "equal dual input uses canonical",
			canonical: canonical,
			legacy:    filepath.Join(canonical, "."),
			want:      canonical,
		},
		{
			name:      "conflicting dual input fails",
			canonical: canonical,
			legacy:    legacy,
			wantError: []string{catalogroot.Env, canonical, catalogroot.LegacyEnv, legacy},
		},
		{
			name:      "relative canonical resolves from startup directory",
			canonical: "canonical",
			want:      canonical,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(catalogroot.Env, test.canonical)
			t.Setenv(catalogroot.LegacyEnv, test.legacy)
			before, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolveCatalogRoot("coding-agent focused test", startup)
			if len(test.wantError) > 0 {
				if err == nil {
					t.Fatal("expected catalog-root conflict")
				}
				for _, value := range test.wantError {
					if !strings.Contains(err.Error(), value) {
						t.Errorf("error %q does not contain %q", err, value)
					}
				}
			} else if err != nil {
				t.Fatalf("resolveCatalogRoot: %v", err)
			} else if got != test.want || !filepath.IsAbs(got) {
				t.Fatalf("catalog root = %q, want absolute %q", got, test.want)
			}
			after, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("catalog resolution changed process CWD from %q to %q", before, after)
			}
		})
	}
}

func TestCodingAgentCatalogRootDiscoversSiblingCatalog(t *testing.T) {
	repository := t.TempDir()
	catalog := codingAgentCatalogFixture(t, filepath.Join(repository, "applications", "catalog"))
	owner := filepath.Join(repository, "applications", "coding-agent")
	if err := os.MkdirAll(owner, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(catalogroot.Env, "")
	t.Setenv(catalogroot.LegacyEnv, "")

	got, err := resolveCatalogRoot("coding-agent discovery test", owner)
	if err != nil {
		t.Fatalf("resolveCatalogRoot: %v", err)
	}
	if got != catalog {
		t.Fatalf("catalog root = %q, want %q", got, catalog)
	}
}

func codingAgentCatalogFixture(t *testing.T, root string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "go.mod"),
		"module github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog\n\ngo 1.26.3\n")
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(absolute)
}
