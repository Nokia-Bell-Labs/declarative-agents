// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAuditSubModulesRunsModulesWithMagefiles(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "agent-core", "magefiles"))
	mkdir(t, filepath.Join(root, "agent-profiles", "magefiles"))
	mkdir(t, filepath.Join(root, "design-patterns", "magefiles"))

	var got []string
	err := auditSubModules(
		[]string{
			filepath.Join(root, "agent-core"),
			filepath.Join(root, "agent-profiles"),
			filepath.Join(root, "design-patterns"),
		},
		os.Stat,
		func(dir string) error {
			got = append(got, filepath.Base(dir))
			return nil
		},
	)

	if err != nil {
		t.Fatalf("auditSubModules returned error: %v", err)
	}
	want := []string{"agent-core", "agent-profiles", "design-patterns"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("audited modules = %#v, want %#v", got, want)
	}
}

func TestAuditSubModulesSkipsModulesWithoutMagefiles(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "agent-core", "magefiles"))
	mkdir(t, filepath.Join(root, "no-mage"))

	var got []string
	err := auditSubModules(
		[]string{filepath.Join(root, "agent-core"), filepath.Join(root, "no-mage")},
		os.Stat,
		func(dir string) error {
			got = append(got, filepath.Base(dir))
			return nil
		},
	)

	if err != nil {
		t.Fatalf("auditSubModules returned error: %v", err)
	}
	want := []string{"agent-core"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("audited modules = %#v, want %#v", got, want)
	}
}

func TestAuditSubModulesWrapsRunnerError(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "agent-core", "magefiles"))
	want := errors.New("audit failed")

	err := auditSubModules(
		[]string{filepath.Join(root, "agent-core")},
		os.Stat,
		func(string) error {
			return want
		},
	)

	if !errors.Is(err, want) {
		t.Fatalf("auditSubModules error = %v, want %v", err, want)
	}
	if got := err.Error(); !strings.Contains(got, "audit in "+filepath.Join(root, "agent-core")) {
		t.Fatalf("error = %q, want module context", got)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
