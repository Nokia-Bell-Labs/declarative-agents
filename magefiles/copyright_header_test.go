// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCheckCopyrightHeaderGoYAMLMarkdown(t *testing.T) {
	cases := []struct {
		path    string
		content string
	}{
		{"demo.go", "// Copyright (c) 2026 Nokia\n// SPDX-License-Identifier: BSD-3-Clause\n\npackage demo\n"},
		{"demo.yaml", "# Copyright (c) 2026 Nokia\n# SPDX-License-Identifier: BSD-3-Clause\nname: demo\n"},
		{"demo.yml", "# Copyright (c) 2026 Nokia\n# SPDX-License-Identifier: BSD-3-Clause\nversion: \"2\"\n"},
		{"demo.md", "<!-- Copyright (c) 2026 Nokia -->\n<!-- SPDX-License-Identifier: BSD-3-Clause -->\n\n# Demo\n"},
	}
	for _, tc := range cases {
		if err := checkCopyrightHeader([]byte(tc.content), tc.path); err != nil {
			t.Errorf("%s: %v", tc.path, err)
		}
	}
}

func TestCheckCopyrightHeaderHelmTemplateYAML(t *testing.T) {
	path := "applications/demo/helm/templates/deployment.yaml"
	content := "{{/* Copyright (c) 2026 Nokia */}}\n{{/* SPDX-License-Identifier: BSD-3-Clause */}}\n{{- if .Values.enabled }}\napiVersion: apps/v1\nkind: Deployment\n{{- end }}\n"
	if err := checkCopyrightHeader([]byte(content), path); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCopyrightHeaderMarkdownFrontmatter(t *testing.T) {
	content := "---\ntitle: Demo\n---\n<!-- Copyright (c) 2026 Nokia -->\n<!-- SPDX-License-Identifier: BSD-3-Clause -->\n\n# Body\n"
	if err := checkCopyrightHeader([]byte(content), "demo.md"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCopyrightHeaderRejectsOldAndMITForms(t *testing.T) {
	rejects := []struct {
		path    string
		content string
	}{
		{"demo.go", "// Copyright (c) 2026 Nokia. All" + " rights reserved.\n\npackage demo\n"},
		{"demo.go", "// Copyright (c) 2026 Petar Djukic. All" + " rights reserved.\n// SPDX-License-Identifier: " + "MIT\n\npackage demo\n"},
		{"demo.yaml", "# Copyright (c) 2026 Nokia. All" + " rights reserved.\nname: demo\n"},
		{"demo.md", "<!-- Copyright (c) 2026 Nokia. All" + " rights reserved. -->\n\n# Demo\n"},
		{"demo.md", "# Demo\n"},
		{"applications/demo/helm/templates/deployment.yaml", "# Copyright (c) 2026 Nokia\n# SPDX-License-Identifier: BSD-3-Clause\napiVersion: v1\nkind: ConfigMap\n"},
		{"applications/demo/helm/templates/deployment.yaml", "{{/* Copyright (c) 2026 Nokia */}}\n{{/* SPDX-License-Identifier: " + "MIT */}}\napiVersion: v1\nkind: ConfigMap\n"},
	}
	for _, tc := range rejects {
		if err := checkCopyrightHeader([]byte(tc.content), tc.path); err == nil {
			t.Errorf("%s accepted non-canonical header:\n%s", tc.path, tc.content)
		}
	}
}

func TestEveryTrackedSourceHasCanonicalCopyrightHeader(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkTrackedCopyrightHeaders(root); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCopyrightHeaderFrontmatterWithoutCloserFails(t *testing.T) {
	if err := checkCopyrightHeader([]byte("---\ntitle: Demo\n"), "demo.md"); err == nil {
		t.Fatal("expected error for unclosed frontmatter")
	}
}

func TestTrackedHelmTemplateYAMLInventory(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := trackedCopyrightPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, rel := range paths {
		if isHelmTemplateYAML(rel) {
			got = append(got, rel)
		}
	}
	want := []string{
		"applications/agent-architecture/helm/templates/applier.yaml",
		"applications/agent-architecture/helm/templates/collector.yaml",
		"applications/agent-architecture/helm/templates/curator.yaml",
		"applications/agent-architecture/helm/templates/profiles-configmap.yaml",
		"applications/chatbot-mesh/helm/templates/applier.yaml",
		"applications/chatbot-mesh/helm/templates/chatbot.yaml",
		"applications/chatbot-mesh/helm/templates/collector.yaml",
		"applications/chatbot-mesh/helm/templates/creator.yaml",
		"applications/chatbot-mesh/helm/templates/dolt.yaml",
		"applications/chatbot-mesh/helm/templates/observer.yaml",
		"applications/chatbot-mesh/helm/templates/ollama.yaml",
		"applications/chatbot-mesh/helm/templates/profiles-configmap.yaml",
		"applications/chatbot-mesh/helm/templates/provisioning-workflow-orchestrator.yaml",
		"applications/chatbot-mesh/helm/templates/rag-units.yaml",
		"applications/coding-agent/helm/templates/agents.yaml",
		"applications/coding-agent/helm/templates/applier.yaml",
		"applications/coding-agent/helm/templates/collector.yaml",
		"applications/coding-agent/helm/templates/ollama.yaml",
		"applications/coding-agent/helm/templates/profiles-configmaps.yaml",
		"applications/coding-agent/helm/templates/workspace.yaml",
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tracked Helm template YAML inventory =\n%v\nwant\n%v", got, want)
	}
}

func TestTrackedCopyrightPathsOnlyListsSupportedExtensions(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := trackedCopyrightPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected tracked go/yaml/md files")
	}
	for _, rel := range paths {
		ext := strings.ToLower(filepath.Ext(rel))
		if _, ok := copyrightHeaderByExt[ext]; !ok {
			t.Errorf("unexpected path %s", rel)
		}
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
}
