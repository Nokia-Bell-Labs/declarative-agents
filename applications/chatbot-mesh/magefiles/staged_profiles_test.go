// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStagedProfilesCoverEnabledDeployments proves every profile selected by a
// chart template is declared by a deployment entry in agents/application.yaml.
func TestStagedProfilesCoverEnabledDeployments(t *testing.T) {
	chartDir := findChartDir(t)
	applicationRoot := filepath.Dir(chartDir)
	catalogRoot, err := resolveCatalogRoot("manifest deployment coverage", applicationRoot)
	if err != nil {
		t.Fatal(err)
	}
	composition, err := resolveChatbotComposition(applicationRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateChartProfileReferences(chartDir, composition.manifest); err != nil {
		t.Fatal(err)
	}
}

func TestChartRejectsProfileOutsideApplicationManifest(t *testing.T) {
	chart := filepath.Join(t.TempDir(), "helm")
	if err := copyDirContents(filepath.Join("..", "helm"), chart); err != nil {
		t.Fatal(err)
	}
	template := filepath.Join(chart, "templates", "chatbot.yaml")
	file, err := os.OpenFile(template, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n# /profiles/agents/undeclared/profile.yaml\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	applicationRoot := filepath.Clean("..")
	catalogRoot, err := resolveCatalogRoot("undeclared chart profile test", applicationRoot)
	if err != nil {
		t.Fatal(err)
	}
	composition, err := resolveChatbotComposition(applicationRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	err = validateChartProfileReferences(chart, composition.manifest)
	if err == nil || !strings.Contains(err.Error(), "outside agents/application.yaml") {
		t.Fatalf("undeclared chart profile error = %v", err)
	}
}

func TestPackagingDocNamesManifestAuthority(t *testing.T) {
	chartDir := findChartDir(t)
	doc, err := os.ReadFile(filepath.Join(chartDir, "PACKAGING.md"))
	if err != nil {
		t.Fatalf("read PACKAGING.md: %v", err)
	}
	for _, required := range []string{
		"agents/application.yaml",
		"provenance/application-closure.yaml",
		"transitive closure",
	} {
		if !strings.Contains(string(doc), required) {
			t.Errorf("PACKAGING.md does not document %q", required)
		}
	}
}

func TestStagedProfilesExcludePackagingDocs(t *testing.T) {
	for _, key := range renderedProfileKeys(t) {
		if strings.Contains(key, "PACKAGING.md") {
			t.Errorf("rendered ConfigMap carries documentation key %q; packaging docs are not runtime profile input", key)
		}
	}
}
