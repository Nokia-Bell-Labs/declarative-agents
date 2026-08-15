// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/magefile/mage/mg"
)

var chatbotChartSourceFiles = []string{
	".helmignore",
	"Chart.yaml",
	"PACKAGING.md",
	"README.md",
	"ci/kind-applier-values.yaml",
	"ci/kind-config.yaml",
	"ci/kind-demo-config.yaml",
	"ci/kind-llm-values.yaml",
	"ci/kind-values.yaml",
	"schema-fixtures/README.md",
	"schema-fixtures/bad-missing-rag-description.yaml",
	"schema-fixtures/bad-nresults.yaml",
	"schema-fixtures/bad-rag-name.yaml",
	"schema-fixtures/dup-rag-name.yaml",
	"schema-fixtures/external-missing-url.yaml",
	"schema-fixtures/incluster-missing-chat.yaml",
	"schema-fixtures/valid-add-rag.yaml",
	"templates/NOTES.txt",
	"templates/_chatbot-rest.tpl",
	"templates/_chatbot-topology.tpl",
	"templates/_chatbot-ui.tpl",
	"templates/_helpers.tpl",
	"templates/applier.yaml",
	"templates/chatbot.yaml",
	"templates/collector.yaml",
	"templates/provisioning-workflow-orchestrator.yaml",
	"templates/creator.yaml",
	"templates/dolt.yaml",
	"templates/observer.yaml",
	"templates/ollama.yaml",
	"templates/profiles-configmap.yaml",
	"templates/rag-units.yaml",
	"values.schema.json",
	"values.yaml",
}

// Helm groups operator-facing chart packaging targets.
type Helm mg.Namespace

// Package stages the mesh profiles and UI into an installable Helm archive.
//
// The source chart intentionally does not duplicate the canonical programs under
// agents/. Operators must install this packaged artifact, not the
// unstaged helm/ directory.
func (Helm) Package() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	destination := demoHelmDist(root)
	return packageHelmChart(filepath.Join(root, "helm"), root, destination)
}

func packageHelmChart(chartDir, profilesRoot, destination string) error {
	if _, err := exec.LookPath("helm"); err != nil {
		return fmt.Errorf("package chatbot-mesh chart: helm not found on PATH")
	}
	catalogRoot, err := resolveCatalogRoot("chatbot-mesh helm package", profilesRoot)
	if err != nil {
		return err
	}
	staged, cleanup, err := stagePackageChart(chartDir, profilesRoot, catalogRoot)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := validateStagedChatbotChart(staged); err != nil {
		return err
	}
	expected, err := regularChartFiles(staged)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create Helm package destination %s: %w", destination, err)
	}
	if output, err := exec.Command(
		"helm", "package", staged, "--destination", destination).CombinedOutput(); err != nil {
		return fmt.Errorf("package staged chatbot-mesh chart: %w: %s",
			err, strings.TrimSpace(string(output)))
	}
	archive := filepath.Join(destination, "chatbot-mesh-0.1.0.tgz")
	if err := validateChatbotChartArchive(archive, expected); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"lint", archive},
		{"template", "packaged", archive},
	} {
		if output, err := exec.Command("helm", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("helm %s: %w: %s",
				strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	fmt.Printf("helm:package: install the validated chart %s\n", archive)
	return nil
}

func stagePackageChart(chartDir, profilesRoot, catalogRoot string) (string, func(), error) {
	stage, err := os.MkdirTemp("", "chatbot-mesh-package-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	chart := filepath.Join(stage, "chatbot-mesh")
	if err := stageChatbotChartSource(chartDir, chart); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := stageChatbotComposition(chart, profilesRoot, catalogRoot); err != nil {
		cleanup()
		return "", nil, err
	}
	return chart, cleanup, nil
}

func stageChatbotChartSource(source, destination string) error {
	actual, err := regularChartFilesExcludingGenerated(source)
	if err != nil {
		return err
	}
	expected := append([]string(nil), chatbotChartSourceFiles...)
	sort.Strings(expected)
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		return fmt.Errorf("chart source inventory mismatch:\nactual: %s\nexpected: %s",
			strings.Join(actual, ", "), strings.Join(expected, ", "))
	}
	for _, relative := range expected {
		if err := copyChartFileStrict(
			filepath.Join(source, filepath.FromSlash(relative)),
			filepath.Join(destination, filepath.FromSlash(relative)),
		); err != nil {
			return err
		}
	}
	return nil
}

func copyChartFileStrict(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("chart source file %s is not regular", source)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, data, info.Mode().Perm())
}

func regularChartFilesExcludingGenerated(root string) ([]string, error) {
	return regularChartFilesWithFilter(root, func(relative string, entry os.DirEntry) bool {
		first := strings.SplitN(relative, "/", 2)[0]
		return first == "dist" || first == "profiles"
	})
}

func regularChartFiles(root string) ([]string, error) {
	return regularChartFilesWithFilter(root, nil)
}

func regularChartFilesWithFilter(
	root string,
	exclude func(string, os.DirEntry) bool,
) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if exclude != nil && exclude(relative, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("chart source contains link %s", relative)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("chart source contains non-regular file %s", relative)
		}
		files = append(files, relative)
		return nil
	})
	sort.Strings(files)
	return files, err
}

func validateStagedChatbotChart(chart string) error {
	commands := [][]string{
		{"lint", chart},
		{"template", "validation", chart},
		{"template", "validation", chart, "-f", filepath.Join(chart, "ci", "kind-values.yaml")},
		{"template", "validation", chart, "-f", filepath.Join(chart, "ci", "kind-llm-values.yaml")},
		{"template", "validation", chart,
			"-f", filepath.Join(chart, "ci", "kind-values.yaml"),
			"-f", filepath.Join(chart, "ci", "kind-applier-values.yaml")},
		{"template", "validation", chart,
			"-f", filepath.Join(chart, "schema-fixtures", "valid-add-rag.yaml")},
	}
	for _, args := range commands {
		if output, err := exec.Command("helm", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("helm %s: %w: %s",
				strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func validateChatbotChartArchive(archive string, stagedFiles []string) error {
	required := make(map[string]bool, len(stagedFiles))
	for _, path := range stagedFiles {
		required["chatbot-mesh/"+filepath.ToSlash(path)] = true
	}
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open chart archive: %w", err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open chart gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read chart archive: %w", err)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return fmt.Errorf("chart archive contains link %s", header.Name)
		}
		if header.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(header.Name)
		if !required[name] {
			return fmt.Errorf("chart archive contains unexpected file %s", name)
		}
		delete(required, name)
	}
	if len(required) > 0 {
		var missing []string
		for path := range required {
			missing = append(missing, path)
		}
		sort.Strings(missing)
		return fmt.Errorf("chart archive missing required files: %s", strings.Join(missing, ", "))
	}
	return nil
}
