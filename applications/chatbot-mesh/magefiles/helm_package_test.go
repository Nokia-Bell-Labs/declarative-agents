// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestHelmPackageIsRepeatableAndExcludesGeneratedInputs(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := filepath.Join(t.TempDir(), "helm")
	if err := copyDirContents(filepath.Join("..", "helm"), chart); err != nil {
		t.Fatal(err)
	}
	writePackageTestFile(t, filepath.Join(chart, "dist", "prior.tgz"), "old")
	writePackageTestFile(t, filepath.Join(chart, "profiles", "stale.yaml"), "stale: true\n")
	destination := filepath.Join(chart, "dist")
	profilesRoot := filepath.Clean("..")

	if err := packageHelmChart(chart, profilesRoot, destination); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(destination, "chatbot-mesh-0.1.0.tgz")
	first := chatbotArchiveFileNames(t, archive)
	if err := packageHelmChart(chart, profilesRoot, destination); err != nil {
		t.Fatal(err)
	}
	second := chatbotArchiveFileNames(t, archive)
	if strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Fatalf("repeat package inventory changed:\nfirst=%v\nsecond=%v", first, second)
	}
	for _, name := range second {
		if strings.Contains(name, "chatbot-mesh/dist/") ||
			strings.HasSuffix(name, "/profiles/stale.yaml") ||
			strings.HasSuffix(name, "/prior.tgz") ||
			strings.Contains(name, "/profiles/agents/coordinator/") ||
			strings.HasSuffix(name, "/templates/coordinator.yaml") {
			t.Errorf("archive contains excluded generated input %s", name)
		}
	}
	for _, required := range []string{
		"chatbot-mesh/templates/chatbot.yaml",
		"chatbot-mesh/profiles/agents/chatbot/profile.yaml",
		"chatbot-mesh/profiles/agents/knowledge-manager/corpus-ingest/profile.yaml",
		"chatbot-mesh/profiles/agents/chatbot/ui/app/dist/index.html",
		"chatbot-mesh/profiles/agents/observer/ui/dist/index.html",
	} {
		if !containsArchiveFile(second, required) {
			t.Errorf("archive is missing required runtime asset %s", required)
		}
	}
}

func TestHelmPackageRecordsCatalogCompatibilityPin(t *testing.T) {
	chart := mustReadPackageTestFile(t, filepath.Join("..", "helm", "Chart.yaml"))
	const annotation = "declarative-agents.nokia.com/catalog-compatible-release: applications/catalog/v0.20260730.0"
	if !strings.Contains(string(chart), annotation) {
		t.Fatalf("Chart.yaml missing exact catalog compatibility annotation %q", annotation)
	}
	packaging := mustReadPackageTestFile(t, filepath.Join("..", "helm", "PACKAGING.md"))
	if !strings.Contains(string(packaging), "applications/catalog/v0.20260730.0") ||
		!strings.Contains(string(packaging), "does not create release tags") {
		t.Fatal("PACKAGING.md does not distinguish the exact compatibility pin from post-merge tag publication")
	}
}

func TestHelmPackageRunsFromCopiedStandaloneLayout(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	standalone := filepath.Join(t.TempDir(), "chatbot-mesh")
	for _, dir := range []string{"helm", "agents", "agents/chatbot/ui/app/dist"} {
		if err := copyDirContents(
			filepath.Join("..", filepath.FromSlash(dir)),
			filepath.Join(standalone, filepath.FromSlash(dir))); err != nil {
			t.Fatalf("copy standalone %s: %v", dir, err)
		}
	}
	writePackageTestFile(t, filepath.Join(standalone, "agents", "chatbot", "ui", "ui.yaml"),
		string(mustReadPackageTestFile(t, filepath.Join("..", "agents", "chatbot", "ui", "ui.yaml"))))
	canonicalRoot, err := filepath.Abs(filepath.Join("..", "..", "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	writeDemoConfig(t, standalone, "catalog_root: "+canonicalRoot)

	destination := filepath.Join(standalone, "helm", "dist")
	if err := packageHelmChart(
		filepath.Join(standalone, "helm"), standalone, destination); err != nil {
		t.Fatalf("package copied standalone layout: %v", err)
	}
	files := chatbotArchiveFileNames(
		t, filepath.Join(destination, "chatbot-mesh-0.1.0.tgz"))
	for _, required := range []string{
		"chatbot-mesh/profiles/agents/corpus-ingest/profile.yaml",
		"chatbot-mesh/profiles/agents/knowledge-manager/corpus-ingest/machine.yaml",
	} {
		if !containsArchiveFile(files, required) {
			t.Errorf("standalone archive missing %s", required)
		}
	}
}

func TestChatbotChartSourceInventoryRejectsDrift(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "unexpected file",
			mutate: func(t *testing.T, chart string) {
				writePackageTestFile(t, filepath.Join(chart, "mystery.txt"), "unexpected")
			},
		},
		{
			name: "missing template",
			mutate: func(t *testing.T, chart string) {
				if err := os.Remove(filepath.Join(chart, "templates", "applier.yaml")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "source link",
			mutate: func(t *testing.T, chart string) {
				path := filepath.Join(chart, "values.yaml")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("Chart.yaml", path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chart := filepath.Join(t.TempDir(), "helm")
			if err := copyDirContents(filepath.Join("..", "helm"), chart); err != nil {
				t.Fatal(err)
			}
			tc.mutate(t, chart)
			err := stageChatbotChartSource(chart, filepath.Join(t.TempDir(), "staged"))
			if err == nil {
				t.Fatal("stageChatbotChartSource accepted source inventory drift")
			}
		})
	}
}

func chatbotArchiveFileNames(t *testing.T, archive string) []string {
	t.Helper()
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !header.FileInfo().IsDir() {
			names = append(names, filepath.ToSlash(header.Name))
		}
	}
	sort.Strings(names)
	return names
}

func containsArchiveFile(files []string, want string) bool {
	index := sort.SearchStrings(files, want)
	return index < len(files) && files[index] == want
}

func writePackageTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadPackageTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestHelmPackageContainsRequiredProfileEntrypoints(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	destination := t.TempDir()
	if err := packageHelmChart(chart, filepath.Dir(chart), destination); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(filepath.Join(destination, "chatbot-mesh-*.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != 1 {
		t.Fatalf("packaged archives = %v, want exactly one", archives)
	}
	out, err := exec.Command("helm", "template", "t", archives[0]).CombinedOutput()
	if err != nil {
		t.Fatalf("render packaged chart: %v\n%s", err, out)
	}
	render := string(out)
	for _, key := range []string{
		"agents__chatbot__profile.yaml",
		"agents__rag-server__profile.yaml",
		"agents__provisioning-workflow-orchestrator__profile.yaml",
		"agents__creator__profile.yaml",
		"agents__applier__profile.yaml",
		"agents__corpus-ingest__profile.yaml",
		"agents__knowledge-manager__corpus-ingest__machine.yaml",
		"agents__chatbot__ui__app__dist__index.html",
		"agents__observer__ui__dist__index.html",
	} {
		if !strings.Contains(render, key+": |-") {
			t.Errorf("packaged profiles ConfigMap missing %s", key)
		}
	}
}
