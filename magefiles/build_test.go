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

func TestBuildSubModulesRunsModulesWithMagefiles(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "agent-core", "magefiles"))
	mkdir(t, filepath.Join(root, "agent-profiles", "magefiles"))
	mkdir(t, filepath.Join(root, "design-patterns", "magefiles"))

	var got []string
	err := buildSubModules(
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
		t.Fatalf("buildSubModules returned error: %v", err)
	}
	want := []string{"agent-core", "agent-profiles", "design-patterns"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("built modules = %#v, want %#v", got, want)
	}
}

func TestBuildSubModulesSkipsModulesWithoutMagefiles(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "agent-core", "magefiles"))
	mkdir(t, filepath.Join(root, "no-mage"))

	var got []string
	err := buildSubModules(
		[]string{filepath.Join(root, "agent-core"), filepath.Join(root, "no-mage")},
		os.Stat,
		func(dir string) error {
			got = append(got, filepath.Base(dir))
			return nil
		},
	)

	if err != nil {
		t.Fatalf("buildSubModules returned error: %v", err)
	}
	want := []string{"agent-core"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("built modules = %#v, want %#v", got, want)
	}
}

func TestBuildSubModulesWrapsRunnerError(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "agent-core", "magefiles"))
	want := errors.New("build failed")

	err := buildSubModules(
		[]string{filepath.Join(root, "agent-core")},
		os.Stat,
		func(string) error {
			return want
		},
	)

	if !errors.Is(err, want) {
		t.Fatalf("buildSubModules error = %v, want %v", err, want)
	}
	if got := err.Error(); !strings.Contains(got, "build in "+filepath.Join(root, "agent-core")) {
		t.Fatalf("error = %q, want module context", got)
	}
}
