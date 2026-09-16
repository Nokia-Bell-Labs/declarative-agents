// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateRag1VariantStagesUnderItsRoot is GH-2096: the rag-server profile
// imports ../units/types-server.yaml, so staging it at the temp root itself put
// that unit one level above the root, which profilestage refuses. Staged under
// agents/rag-server, the unit lands inside the root beside it.
func TestGenerateRag1VariantStagesUnderItsRoot(t *testing.T) {
	profilesRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	profile, cleanup, err := generateRag1Variant(profilesRoot)
	if err != nil {
		t.Fatalf("generateRag1Variant: %v", err)
	}
	defer cleanup()

	root := filepath.Dir(filepath.Dir(filepath.Dir(profile)))
	if filepath.Base(filepath.Dir(profile)) != "rag-server" || filepath.Base(filepath.Dir(filepath.Dir(profile))) != "agents" {
		t.Fatalf("profile staged at %s, want <root>/agents/rag-server/profile.yaml", profile)
	}
	if _, err := os.Stat(filepath.Join(root, "agents", "units", "types-server.yaml")); err != nil {
		t.Fatalf("imported unit not staged beside the profile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "units")); !os.IsNotExist(err) {
		t.Fatalf("a units directory exists beside the root %s: nothing may be written outside it", root)
	}
	rest, err := os.ReadFile(filepath.Join(filepath.Dir(profile), "rest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rest), "18085") || !strings.Contains(string(rest), "18095") {
		t.Fatalf("rag1 ports were not rewritten:\n%s", rest)
	}
}
