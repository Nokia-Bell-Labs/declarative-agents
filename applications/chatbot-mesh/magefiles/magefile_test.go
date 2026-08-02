// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpecificationCriticSucceeded pins the report classification to the specification critic's observed
// output contract: a clean run ends "terminal state: succeeded"; a failing run
// ends "terminal state: failed" (with "status=failed" in the run-complete log).
// The classification reads the report rather than the exit code because the
// report names which checks failed, where the exit code only says that some did
// (agent-core srd018 R6, GH-683). A report with neither marker is an
// indeterminate run and must be an error, not a silent pass.
func TestSpecificationCriticSucceeded(t *testing.T) {
	cases := []struct {
		name    string
		report  string
		wantOK  bool
		wantErr bool
	}{
		{
			name:   "clean corpus",
			report: "validate: 3 SRDs ... — OK\nrun complete: status=succeeded\nterminal state: succeeded\n",
			wantOK: true,
		},
		{
			name:    "error finding fails",
			report:  "[error] builtin-spec-corpus/index-broken-path ...\nrun complete: status=failed\nterminal state: failed\n",
			wantOK:  false,
			wantErr: false,
		},
		{
			name:    "status=failed without terminal line still fails",
			report:  "run complete: status=failed iterations=3\n",
			wantOK:  false,
			wantErr: false,
		},
		{
			name:   "warnings only still succeed",
			report: "[warning] builtin-spec-corpus/orphaned-srd ...\nterminal state: succeeded\n",
			wantOK: true,
		},
		{
			name:    "indeterminate run is an error",
			report:  "building agent binary...\n",
			wantOK:  false,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := specificationCriticSucceeded(tc.report)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestResolveAuditToolsRequiresRuntimeAndValidator pins the self-governance gate:
// a copied-out application that cannot reach the agent-core runtime or the specification-critic
// validator profile must fail, not skip to a false green. Only when both tools
// are present does resolution succeed.
func TestResolveAuditToolsRequiresRuntimeAndValidator(t *testing.T) {
	t.Run("missing agent-core runtime fails", func(t *testing.T) {
		root := t.TempDir()
		writeDemoConfig(t, root,
			"core_root: "+filepath.Join(t.TempDir(), "absent-core"),
			"spec_critic_profile: "+writeFile(t, "profile.yaml", "name: fake-specification-critic\n"))
		if _, _, err := resolveAuditTools(root, t.TempDir()); err == nil {
			t.Fatal("expected an error when agent-core is absent, got nil")
		}
	})
	t.Run("missing specification-critic validator fails", func(t *testing.T) {
		root := t.TempDir()
		writeDemoConfig(t, root,
			"core_root: "+fakeCore(t),
			"spec_critic_profile: "+filepath.Join(t.TempDir(), "absent-profile.yaml"))
		if _, _, err := resolveAuditTools(root, t.TempDir()); err == nil {
			t.Fatal("expected an error when the specification-critic validator is absent, got nil")
		}
	})
	t.Run("both present resolves", func(t *testing.T) {
		core := fakeCore(t)
		profile := writeFile(t, "profile.yaml", "name: fake-specification-critic\n")
		catalog := t.TempDir()
		root := t.TempDir()
		writeDemoConfig(t, root, "core_root: "+core, "spec_critic_profile: "+profile)
		coreRoot, specificationCriticProfile, err := resolveAuditTools(root, catalog)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if coreRoot != core || specificationCriticProfile != profile {
			t.Fatalf("resolved (%s, %s), want (%s, %s)", coreRoot, specificationCriticProfile, core, profile)
		}
	})
}

// writeDemoConfig writes a demo.yaml with the given "key: value" lines into the
// application root, so tests drive the magefile resolvers through the declared
// config instead of environment variables.
func writeDemoConfig(t *testing.T, applicationRoot string, lines ...string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(applicationRoot, demoConfigFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveCatalogRootFromDemoConfig(t *testing.T) {
	startup := t.TempDir()
	catalog := filepath.Join(startup, "catalog")
	if err := os.MkdirAll(filepath.Join(catalog, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeAgentFixture(t, filepath.Join(catalog, "go.mod"),
		"module github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog\n")

	tests := []struct {
		name        string
		catalogRoot string
		want        string
	}{
		{name: "absolute catalog_root", catalogRoot: catalog, want: catalog},
		{name: "relative catalog_root resolves from startup directory", catalogRoot: "catalog", want: catalog},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeDemoConfig(t, startup, "catalog_root: "+test.catalogRoot)
			before, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolveCatalogRoot("chatbot test", startup)
			if err != nil || got != test.want {
				t.Fatalf("resolveCatalogRoot = %q, %v; want %q", got, err, test.want)
			}
			after, getwdErr := os.Getwd()
			if getwdErr != nil || after != before {
				t.Fatalf("process CWD changed from %q to %q (%v)", before, after, getwdErr)
			}
		})
	}
}

func TestChatbotReadmeResolvesCanonicalPuppeteerOwner(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	const relativePackage = "../catalog/agents/knowledge-manager/documentation-curator/ui/docs/"
	text := string(readme)
	for _, required := range []string{
		relativePackage,
		"`npm ci`",
		"`npm run test:e2e:machine-request`",
		"`PUPPETEER_EXECUTABLE_PATH`",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("chatbot README missing Puppeteer instruction %q", required)
		}
	}
	if strings.Contains(text, "puppeteer-core` install lives in agent-core") {
		t.Fatal("chatbot README retains stale agent-core Puppeteer ownership")
	}

	packagePath := filepath.Clean(filepath.Join("..", filepath.FromSlash(relativePackage)))
	data, err := os.ReadFile(filepath.Join(packagePath, "package.json"))
	if err != nil {
		t.Fatalf("canonical Puppeteer package does not resolve: %v", err)
	}
	var pkg struct {
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Dependencies["puppeteer-core"] == "" &&
		pkg.DevDependencies["puppeteer-core"] == "" {
		t.Fatal("canonical package no longer owns puppeteer-core")
	}
	if pkg.Scripts["test:e2e:machine-request"] == "" {
		t.Fatal("documented Puppeteer E2E script no longer exists")
	}
}

// fakeCore returns a temp directory that agentCoreAvailable accepts as an
// agent-core module checkout (it carries a go.mod file).
func fakeCore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeFile writes content to a named file in a fresh temp directory and returns
// its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
