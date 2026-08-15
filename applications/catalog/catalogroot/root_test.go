// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalogroot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveExplicitRoot(t *testing.T) {
	cwd := t.TempDir()
	explicit := catalogFixture(t)

	tests := []struct {
		name        string
		catalogRoot string
		wantPath    string
		wantSource  Source
		wantError   []string
	}{
		{
			name: "explicit valid root", catalogRoot: explicit,
			wantPath: explicit, wantSource: SourceExplicit,
		},
		{
			name:        "explicit invalid does not fall back",
			catalogRoot: filepath.Join(cwd, "missing"),
			wantError:   []string{"catalog_root", filepath.Join(cwd, "missing"), "invalid catalog root"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Resolve("test command", cwd, test.catalogRoot)
			if len(test.wantError) > 0 {
				requireErrorContains(t, err, test.wantError...)
				return
			}
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if got.Path != test.wantPath || got.Source != test.wantSource {
				t.Fatalf("Resolve = %#v, want path=%q source=%q", got, test.wantPath, test.wantSource)
			}
		})
	}
}

func TestResolveRelativeInputsAgainstStartupWorkingDirectory(t *testing.T) {
	cwd := t.TempDir()
	root := filepath.Join(cwd, "catalog")
	writeCatalogFixture(t, root)
	processCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	got, err := Resolve("relative test", cwd, "catalog")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.Path != root || !filepath.IsAbs(got.Path) {
		t.Fatalf("Path = %q, want absolute %q", got.Path, root)
	}
	after, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if after != processCWD {
		t.Fatalf("Resolve changed process CWD from %q to %q", processCWD, after)
	}
}

func TestResolveDiscoversApplicationsCatalog(t *testing.T) {
	repository := t.TempDir()
	root := filepath.Join(repository, "applications", "catalog")
	writeCatalogFixture(t, root)
	owner := filepath.Join(repository, "applications", "coding-agent")
	if err := os.MkdirAll(owner, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve("coding-agent package", owner, "", DiscoveryCandidates(owner)...)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.Path != root || got.Source != SourceDiscovery {
		t.Fatalf("Resolve = %#v, want discovered %q", got, root)
	}
	if got.AgentsRoot() != filepath.Join(root, "agents") {
		t.Fatalf("AgentsRoot = %q", got.AgentsRoot())
	}
	if got.ConformanceRoot() != filepath.Join(root, "testdata", "conformance") {
		t.Fatalf("ConformanceRoot = %q", got.ConformanceRoot())
	}
}

func TestResolveInstalledRuntimeAndMissingCatalogDiagnostics(t *testing.T) {
	installed := filepath.Join(t.TempDir(), "opt", "agent-core")
	if err := os.MkdirAll(installed, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve("installed runtime", installed, "", DiscoveryCandidates(installed)...)
	requireErrorContains(t, err, "installed runtime", "catalog_root", "applications/catalog")

	attempted := filepath.Join(installed, "explicit-missing")
	_, err = Resolve("catalog conformance", installed, attempted)
	requireErrorContains(t, err, "catalog conformance", "catalog_root", attempted, "invalid catalog root")
}

func TestDiscoveryCandidatesNeverNameLegacySourceRoot(t *testing.T) {
	for _, candidate := range DiscoveryCandidates(filepath.Join(t.TempDir(), "applications", "coding-agent")) {
		if filepath.Base(candidate) == "agent-profiles" {
			t.Fatalf("legacy source-root candidate returned: %s", candidate)
		}
	}
}

func catalogFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeCatalogFixture(t, root)
	return root
}

func writeCatalogFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "module " + catalogModule + "\n\ngo 1.26.3\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func requireErrorContains(t *testing.T, err error, values ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, value := range values {
		if !strings.Contains(err.Error(), value) {
			t.Fatalf("error %q does not contain %q", err, value)
		}
	}
}
