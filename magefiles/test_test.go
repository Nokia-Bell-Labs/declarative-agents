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

func TestUnitSubModulesRunsGoModulesInOrder(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, filepath.Join(root, "agent-core"))
	writeGoMod(t, filepath.Join(root, "agent-profiles"))
	mkdir(t, filepath.Join(root, "design-patterns"))

	var got []string
	err := testUnitSubModules(
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
		t.Fatalf("testUnitSubModules returned error: %v", err)
	}
	want := []string{"agent-core", "agent-profiles"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unit-tested modules = %#v, want %#v", got, want)
	}
}

func TestSubModulesRunsModulesWithGoTests(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, filepath.Join(root, "agent-core"))
	writeGoMod(t, filepath.Join(root, "agent-profiles"))
	writeFile(t, filepath.Join(root, "agent-core", "magefiles", "build_test.go"), "package main\n")
	writeFile(t, filepath.Join(root, "agent-profiles", "magefiles", "validation_test.go"), "package main\n")
	mkdir(t, filepath.Join(root, "design-patterns"))

	var got []string
	err := testSubModules(
		[]string{
			filepath.Join(root, "agent-core"),
			filepath.Join(root, "agent-profiles"),
			filepath.Join(root, "design-patterns"),
		},
		moduleHasGoTests,
		func(dir string) error {
			got = append(got, filepath.Base(dir))
			return nil
		},
	)

	if err != nil {
		t.Fatalf("testSubModules returned error: %v", err)
	}
	want := []string{"agent-core", "agent-profiles"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tested modules = %#v, want %#v", got, want)
	}
}

func TestSubModulesWrapsRunnerError(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, filepath.Join(root, "agent-core"))
	writeFile(t, filepath.Join(root, "agent-core", "magefiles", "build_test.go"), "package main\n")
	want := errors.New("mage test failed")

	err := testSubModules(
		[]string{filepath.Join(root, "agent-core")},
		moduleHasGoTests,
		func(string) error {
			return want
		},
	)

	if !errors.Is(err, want) {
		t.Fatalf("testSubModules error = %v, want %v", err, want)
	}
	if got := err.Error(); !strings.Contains(got, "tests in "+filepath.Join(root, "agent-core")) {
		t.Fatalf("error = %q, want module context", got)
	}
}

func TestUnitSubModulesSkipsModulesWithoutGoMod(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, filepath.Join(root, "agent-core"))
	mkdir(t, filepath.Join(root, "design-patterns"))

	var got []string
	err := testUnitSubModules(
		[]string{filepath.Join(root, "agent-core"), filepath.Join(root, "design-patterns")},
		os.Stat,
		func(dir string) error {
			got = append(got, filepath.Base(dir))
			return nil
		},
	)

	if err != nil {
		t.Fatalf("testUnitSubModules returned error: %v", err)
	}
	want := []string{"agent-core"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unit-tested modules = %#v, want %#v", got, want)
	}
}

func TestUnitSubModulesWrapsRunnerError(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, filepath.Join(root, "agent-core"))
	want := errors.New("go test failed")

	err := testUnitSubModules(
		[]string{filepath.Join(root, "agent-core")},
		os.Stat,
		func(string) error {
			return want
		},
	)

	if !errors.Is(err, want) {
		t.Fatalf("testUnitSubModules error = %v, want %v", err, want)
	}
	if got := err.Error(); !strings.Contains(got, "unit tests in "+filepath.Join(root, "agent-core")) {
		t.Fatalf("error = %q, want module context", got)
	}
}

func writeGoMod(t *testing.T, dir string) {
	t.Helper()
	mkdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.invalid/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
