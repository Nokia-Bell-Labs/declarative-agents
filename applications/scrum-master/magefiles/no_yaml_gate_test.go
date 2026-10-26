// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
)

// notMarkdown are the files design.md D2 keeps out of Markdown, each because
// a consumer outside this repository requires its format.
var notMarkdown = []string{
	"docs/external-checkouts.yaml", // the audit reads it by path
	"docs/corpus-charter.yaml",     // the audit executes it
	"plugin/plugin.json",           // the Agent Plugins 1.0 standard
	".golangci.yml",                // the repository's lint gate reads it by path
}

// agentsDir holds the declarative-agents profiles, which the pinned runtime
// reads as YAML (design.md D2, D14; GH-135).
const agentsDir = "plugin/agents/"

// yamlOutsideAllowed returns the YAML and JSON files among paths that D2
// does not allow.
func yamlOutsideAllowed(paths []string) []string {
	var out []string
	for _, p := range paths {
		switch {
		case !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") && !strings.HasSuffix(p, ".json"):
		case slices.Contains(notMarkdown, p):
		case strings.HasPrefix(p, agentsDir) && strings.HasSuffix(p, ".yaml"):
		default:
			out = append(out, p)
		}
	}
	return out
}

// TestNoYAMLOutsideTheAllowedFiles holds the repository to design.md D2
// (srd040 R2.1):
// every tracked file is Markdown with tags except the files notMarkdown
// names.
func TestNoYAMLOutsideTheAllowedFiles(t *testing.T) {
	out, err := exec.Command("git", "-C", "..", "ls-files").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range yamlOutsideAllowed(strings.Split(strings.TrimSpace(string(out)), "\n")) {
		t.Errorf("%s is not Markdown; design.md D2 allows only %s", p, strings.Join(notMarkdown, ", "))
	}
	planted := []string{"docs/VISION.md", "control/policy.yaml", "ci/code.gitlab-ci.yml", "plugin/plugin.json", "x/settings.json", "plugin/agents/implement/machine.yaml", "plugin/agents/implement/plugin.json"}
	if got := yamlOutsideAllowed(planted); !slices.Equal(got, []string{"control/policy.yaml", "ci/code.gitlab-ci.yml", "x/settings.json", "plugin/agents/implement/plugin.json"}) {
		t.Fatalf("planted files found as %v", got)
	}
}

// TestConfigDocumentsRead reads every config document this repository
// holds (design.md D11, srd040 R2.2): the gates file and each config
// format's example.
func TestConfigDocumentsRead(t *testing.T) {
	if _, err := release.LoadGates(filepath.Join("..", release.DefaultGatesFile)); err != nil {
		t.Error(err)
	}
	formats, err := filepath.Glob(filepath.Join("..", "docs", "specs", "config-formats", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range formats {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		_, rest, ok := strings.Cut(string(b), "<!-- field: config_file_layout -->")
		if !ok {
			continue
		}
		_, rest, _ = strings.Cut(rest, "```markdown\n")
		doc, _, _ := strings.Cut(rest, "\n```")
		var v map[string]any
		if err := corpus.DecodeConfig([]byte(doc+"\n"), &v); err != nil || len(v) == 0 {
			t.Errorf("%s: example does not read as a config document: %v", f, err)
		}
	}
}
