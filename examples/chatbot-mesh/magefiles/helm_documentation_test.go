// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHelmReadmeInstallsOnlyPackagedArtifacts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(findChartDir(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	sourceInstall := regexp.MustCompile(`(?m)^\s*helm install\b[^\n]*\b(?:examples/chatbot-mesh/)?helm(?:\s|$)`)
	if command := sourceInstall.FindString(readme); command != "" {
		t.Fatalf("README installs the unstaged source chart: %s", command)
	}
	for _, required := range []string{
		"mage helm:package",
		"helm/dist/chatbot-mesh-*.tgz",
		"--set ollama.enabled=false",
		"--set llm.externalURL=http://my-ollama:11434",
	} {
		if !strings.Contains(readme, required) {
			t.Errorf("README external-Ollama workflow is missing %q", required)
		}
	}
}
