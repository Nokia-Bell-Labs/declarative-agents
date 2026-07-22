// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// agentProfileRefRE matches the two ways the integration magefiles name an agent
// profile: the filepath.Join form ("agents", "critic") and the slash form
// (agents/critic/profile.yaml).
var agentProfileRefRE = regexp.MustCompile(`"agents",\s*"([a-z0-9-]+)"|agents/([a-z0-9-]+)/profile\.yaml`)

// TestIntegrationProfilePathsResolve proves every agent profile path referenced
// by the integration magefiles resolves to a shipped profile. It is the GH-498
// regression guard: the bench and evaluator-generator targets launched the
// removed agents/evaluator/profile.yaml after the rel10 evaluator->critic rename.
func TestIntegrationProfilePathsResolve(t *testing.T) {
	root := repoRootFromTest(t)
	entries, err := filepath.Glob(filepath.Join("*.go"))
	if err != nil {
		t.Fatal(err)
	}

	referenced := map[string]bool{}
	for _, f := range entries {
		if len(f) > len("_test.go") && f[len(f)-len("_test.go"):] == "_test.go" {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range agentProfileRefRE.FindAllStringSubmatch(string(data), -1) {
			name := m[1]
			if name == "" {
				name = m[2]
			}
			referenced[name] = true
		}
	}
	if len(referenced) == 0 {
		t.Fatal("no agent profile references found; regex or layout changed")
	}

	var missing []string
	for name := range referenced {
		p := filepath.Join(root, "agents", name, "profile.yaml")
		if _, err := os.Stat(p); err != nil {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("integration magefiles reference agent profiles that do not exist: %v", missing)
	}
}

// repoRootFromTest walks up from the working directory to the agent-profiles
// root (the directory that contains agents/).
func repoRootFromTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "agents")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("agent-profiles root not found walking up from the test directory")
		}
		dir = parent
	}
}
