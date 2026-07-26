// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestOllamaRESTFixtureMirrorMatchesCanonicalProfile(t *testing.T) {
	t.Parallel()
	coreRoot := RequireCoreRoot(t)
	canonicalRoot := ProfilePath(filepath.Join("testdata", "conformance", "rest"))
	mirrorRoot := filepath.Join(coreRoot, "internal", "tools", "rest", "testdata", "ollama_profile")
	files := []string{
		"ollama-declarations.yaml",
		"ollama-llm.yaml",
		"ollama-machine.yaml",
		"ollama-profile.yaml",
		"ollama-rest.yaml",
		"ollama-tools.yaml",
		filepath.Join("openapi", "ollama.yaml"),
	}

	for _, rel := range files {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			canonical := readFixtureFile(t, filepath.Join(canonicalRoot, rel))
			mirror := readFixtureFile(t, filepath.Join(mirrorRoot, rel))
			if !bytes.Equal(canonical, mirror) {
				t.Fatalf(
					"agent-core Ollama fixture mirror %s drifted from agent-profiles canonical copy (canonical sha256=%x, mirror sha256=%x)",
					rel, sha256.Sum256(canonical), sha256.Sum256(mirror),
				)
			}
		})
	}
}

func readFixtureFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(fmt.Errorf("read fixture %s: %w", path, err))
	}
	return data
}
