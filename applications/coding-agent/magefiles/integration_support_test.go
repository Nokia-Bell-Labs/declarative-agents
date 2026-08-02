// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodingAgentCatalogRootFromDemoConfig(t *testing.T) {
	startup := t.TempDir()
	canonical := codingAgentCatalogFixture(t, filepath.Join(startup, "canonical"))

	tests := []struct {
		name        string
		catalogRoot string
		want        string
	}{
		{name: "absolute catalog_root", catalogRoot: canonical, want: canonical},
		{name: "relative catalog_root resolves from startup directory", catalogRoot: "canonical", want: canonical},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeCodingAgentDemoConfig(t, startup, "catalog_root: "+test.catalogRoot)
			before, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolveCatalogRoot("coding-agent focused test", startup)
			if err != nil {
				t.Fatalf("resolveCatalogRoot: %v", err)
			}
			if got != test.want || !filepath.IsAbs(got) {
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

	got, err := resolveCatalogRoot("coding-agent discovery test", owner)
	if err != nil {
		t.Fatalf("resolveCatalogRoot: %v", err)
	}
	if got != catalog {
		t.Fatalf("catalog root = %q, want %q", got, catalog)
	}
}

func TestCodingAgentCatalogRootDiscoversFromRelativeStartupDir(t *testing.T) {
	repository, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	catalog := codingAgentCatalogFixture(t, filepath.Join(repository, "applications", "catalog"))
	magefiles := filepath.Join(repository, "applications", "coding-agent", "magefiles")
	if err := os.MkdirAll(magefiles, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(magefiles)

	got, err := resolveCatalogRoot("coding-agent relative discovery test", "..")
	if err != nil {
		t.Fatalf("resolveCatalogRoot: %v", err)
	}
	if got != catalog {
		t.Fatalf("catalog root = %q, want %q", got, catalog)
	}
}

// writeCodingAgentDemoConfig writes a demo.yaml with the given "key: value"
// lines into the application root, so tests drive catalog-root resolution
// through the declared config instead of environment variables.
func writeCodingAgentDemoConfig(t *testing.T, applicationRoot string, lines ...string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(applicationRoot, demoConfigFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
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
