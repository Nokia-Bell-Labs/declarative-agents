// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanArtifactsPresentAndAbsent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		present bool
	}{
		{name: "present artifacts", present: true},
		{name: "absent directories"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			figures := filepath.Join(root, "figures")
			output := filepath.Join(root, "generated")
			if tt.present {
				requireWriteFile(t, filepath.Join(figures, "diagram.png"))
				requireWriteFile(t, filepath.Join(figures, "source.puml"))
				requireWriteFile(t, filepath.Join(output, "paper.pdf"))
			}

			if err := cleanArtifacts(figures, output, filepath.Glob, os.ReadDir, os.Remove); err != nil {
				t.Fatalf("cleanArtifacts() error: %v", err)
			}
			if !tt.present {
				return
			}
			if _, err := os.Stat(filepath.Join(figures, "diagram.png")); !os.IsNotExist(err) {
				t.Errorf("generated PNG remains: %v", err)
			}
			if _, err := os.Stat(filepath.Join(output, "paper.pdf")); !os.IsNotExist(err) {
				t.Errorf("generated PDF remains: %v", err)
			}
			if _, err := os.Stat(filepath.Join(figures, "source.puml")); err != nil {
				t.Errorf("source diagram was removed: %v", err)
			}
		})
	}
}

func TestCleanArtifactsAggregatesErrorsAndContinues(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	figures := filepath.Join(root, "figures")
	output := filepath.Join(root, "generated")
	for _, path := range []string{
		filepath.Join(figures, "bad.png"),
		filepath.Join(figures, "good.png"),
		filepath.Join(output, "bad.pdf"),
		filepath.Join(output, "good.pdf"),
	} {
		requireWriteFile(t, path)
	}
	remove := func(path string) error {
		if strings.HasPrefix(filepath.Base(path), "bad.") {
			return errors.New("injected remove failure")
		}
		return os.Remove(path)
	}

	err := cleanArtifacts(figures, output, filepath.Glob, os.ReadDir, remove)
	if err == nil {
		t.Fatal("cleanArtifacts() expected aggregated error")
	}
	for _, name := range []string{"bad.png", "bad.pdf"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
	for _, path := range []string{filepath.Join(figures, "good.png"), filepath.Join(output, "good.pdf")} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("successful cleanup path remains %s: %v", path, statErr)
		}
	}
}

func TestCleanArtifactsAggregatesDiscoveryErrors(t *testing.T) {
	t.Parallel()
	globErr := errors.New("glob failed")
	readErr := errors.New("read failed")
	err := cleanArtifacts(
		"figures",
		"generated",
		func(string) ([]string, error) { return nil, globErr },
		func(string) ([]os.DirEntry, error) { return nil, readErr },
		os.Remove,
	)
	if !errors.Is(err, globErr) || !errors.Is(err, readErr) {
		t.Fatalf("cleanArtifacts() error = %v, want both discovery errors", err)
	}
}

func requireWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
