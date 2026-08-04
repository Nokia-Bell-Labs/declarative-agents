// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAuditApplicationParsesManifestAndDocumentation(t *testing.T) {
	root := realApplicationRoot(t)
	count, err := auditApplication(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 21 {
		t.Fatalf("audited documentation YAML = %d, want 21", count)
	}
}

func TestTracerManifestRejectsAuditOnlyRegression(t *testing.T) {
	root := realApplicationRoot(t)
	manifest, err := loadApplicationManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	capability := manifest.Capabilities["runnable_module"]
	capability.Status = "audit_only"
	capability.Evidence = nil
	manifest.Capabilities["runnable_module"] = capability
	if err := validateTracerManifest(manifest); err == nil ||
		!strings.Contains(err.Error(), "runnable_module") {
		t.Fatalf("validateTracerManifest error = %v, want audit-only regression rejection", err)
	}
}

func TestAuditYAMLDocumentsRejectsMalformedDocument(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "docs", "broken.yaml"), "broken: [\n")
	writeTestFile(t, filepath.Join(root, applicationManifest), "schema_version: 1\n")
	writeTestFile(t, filepath.Join(root, demoConfigFile), "{}\n")
	writeTestFile(t, filepath.Join(root, ".golangci.yml"), "version: 2\n")
	if _, err := auditYAMLDocuments(root); err == nil ||
		!strings.Contains(err.Error(), "broken.yaml") {
		t.Fatalf("auditYAMLDocuments error = %v, want malformed document path", err)
	}
}

func TestStatsReportRunnableAgentsAndExactComposition(t *testing.T) {
	root := realApplicationRoot(t)
	manifest, err := loadApplicationManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := newStats(root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Application.Ownership != "agent-owning" ||
		stats.Application.ModuleStatus != "implemented" ||
		stats.Application.AgentsContributed != 3 {
		t.Fatalf("application stats identity = %#v", stats.Application)
	}
	if stats.Application.CanonicalReferences != 1 ||
		stats.Application.CompositionWrappers != 1 ||
		!reflect.DeepEqual(stats.Application.CanonicalProfiles, []string{
			"applications/catalog/agents/knowledge-manager/corpus-reader/profile.yaml",
		}) {
		t.Fatalf("composition references = %#v, want one corpus-reader wrapper",
			stats.Application)
	}
	wantRoots := append([]string(nil), executableLocalRoots...)
	sortStrings(wantRoots)
	if !reflect.DeepEqual(stats.Application.ExecutableRoots, wantRoots) {
		t.Fatalf("executable roots = %v, want %v", stats.Application.ExecutableRoots, wantRoots)
	}
	if stats.Agents.Total.Agents != 3 || len(stats.Agents.PerAgent) != 3 {
		t.Fatalf("runnable agent stats = %#v", stats.Agents)
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document["agents"]; !exists {
		t.Fatal("runnable Prose Editor stats are missing the agents section")
	}
}

func TestMageSurfaceHasNoRuntimeTargets(t *testing.T) {
	source, err := os.ReadFile("magefile.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	for _, target := range []string{
		"func Run(",
		"func Build(",
		"func Package(",
		"func Helm(",
		"func Integration(",
		"func Test(",
	} {
		if strings.Contains(content, target) {
			t.Errorf("magefile declares unsupported target signature %q", target)
		}
	}
}

func realApplicationRoot(t *testing.T) string {
	t.Helper()
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sortStrings(values []string) {
	for index := 1; index < len(values); index++ {
		for current := index; current > 0 && values[current] < values[current-1]; current-- {
			values[current], values[current-1] = values[current-1], values[current]
		}
	}
}
