// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
)

// The Ollama fixtures once imported tools/builtin/llm/all.yaml and used three
// of its six units, which the loader refuses as a partial import; the targets
// skip without a live Ollama, so nothing caught it (GH-2099). Both profiles
// now load against the real tree with no service running.

func TestOllamaRestProfileLoads(t *testing.T) {
	rootDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	if err := writeOllamaTempProfile(rootDir, tmpDir, "qwen-test", profilePath); err != nil {
		t.Fatal(err)
	}
	if _, err := load.LoadClosure(profilePath, load.Options{CoreRoot: rootDir}); err != nil {
		t.Fatalf("ollama-rest profile does not load: %v", err)
	}
}

func TestOllamaMonitorProfileLoads(t *testing.T) {
	rootDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	run, cleanup, err := prepareMonitoredQwenRun(rootDir, "qwen-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	if _, err := os.Stat(run.profilePath); err != nil {
		t.Fatal(err)
	}
	if _, err := load.LoadClosure(run.profilePath, load.Options{CoreRoot: rootDir}); err != nil {
		t.Fatalf("ollama-monitor profile does not load: %v", err)
	}
}
