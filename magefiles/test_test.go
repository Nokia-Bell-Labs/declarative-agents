// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
	writeGoMod(t, filepath.Join(root, "applications", "catalog"))
	mkdir(t, filepath.Join(root, "design-patterns"))

	var got []string
	err := testUnitSubModules(
		[]string{
			filepath.Join(root, "agent-core"),
			filepath.Join(root, "applications", "catalog"),
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
	want := []string{"agent-core", "catalog"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unit-tested modules = %#v, want %#v", got, want)
	}
}

func TestSubModulesRunsModulesWithGoTests(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, filepath.Join(root, "agent-core"))
	writeGoMod(t, filepath.Join(root, "applications", "catalog"))
	writeFile(t, filepath.Join(root, "agent-core", "magefiles", "build_test.go"), "package main\n")
	writeFile(t, filepath.Join(root, "applications", "catalog", "magefiles", "validation_test.go"), "package main\n")
	mkdir(t, filepath.Join(root, "design-patterns"))

	var got []string
	err := testSubModules(
		[]string{
			filepath.Join(root, "agent-core"),
			filepath.Join(root, "applications", "catalog"),
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
	want := []string{"agent-core", "catalog"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tested modules = %#v, want %#v", got, want)
	}
}

func TestCatalogTestTargetUsesMageRunner(t *testing.T) {
	for _, target := range testTargets() {
		if target.module != "applications/catalog" {
			continue
		}
		if reflect.ValueOf(target.run).Pointer() != reflect.ValueOf(runMageTest).Pointer() {
			t.Fatal("catalog test target does not use its stable-binary Mage runner")
		}
		return
	}
	t.Fatal("catalog test target is not registered")
}

func TestFullSuiteRunnerMapsAgentCoreToMageFull(t *testing.T) {
	for _, target := range testTargets() {
		got := fullSuiteRunner(target)
		switch {
		case target.module == agentCoreModule:
			if !sameRunner(got, runMageTestFull) {
				t.Fatalf("%s full runner is not mage test:full", target.module)
			}
		case sameRunner(target.run, runMageTest):
			if !sameRunner(got, runMageTest) {
				t.Fatalf("%s full runner dropped mage test", target.module)
			}
		default:
			if !sameRunner(got, runGoFullTests) {
				t.Fatalf("%s full runner is not go test without -short", target.module)
			}
		}
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

// TestEveryMaintainedGoModuleIsDispatchedExactlyOnce is the orchestration guard:
// every non-fixture Go module in the repository must be dispatched by the root
// Test gate exactly once, except explicitly audit-only applications whose local
// Go exists solely to implement their root-dispatched audit/stats surface.
func TestEveryMaintainedGoModuleIsDispatchedExactlyOnce(t *testing.T) {
	modules, err := discoverMaintainedGoModules("..")
	if err != nil {
		t.Fatalf("discoverMaintainedGoModules: %v", err)
	}
	if len(modules) == 0 {
		t.Fatal("discovered no maintained Go modules; discovery is broken")
	}

	dispatch := map[string]int{}
	for _, target := range testTargets() {
		dispatch[filepath.ToSlash(target.module)]++
	}

	for _, mod := range modules {
		want := 1
		if contains(auditOnlyApplicationModules, mod) {
			want = 0
		}
		if got := dispatch[mod]; got != want {
			t.Errorf("maintained Go module %q dispatched %d time(s) by the Test gate, want %d", mod, got, want)
		}
	}

	for _, module := range auditOnlyApplicationModules {
		if !contains(modules, module) {
			t.Fatalf("discovery missed audit-only module %q", module)
		}
		if dispatch[module] != 0 {
			t.Fatalf("audit-only module %q entered runnable Test dispatch", module)
		}
	}

	// Guard the specific nested modules that motivated the fix so a regression
	// that drops them from the registry fails loudly.
	for _, nested := range []string{"agent-core/magefiles", "design-patterns/magefiles", "magefiles"} {
		if !contains(modules, nested) {
			t.Fatalf("discovery missed nested module %q; expected it under the repo root", nested)
		}
		if dispatch[nested] != 1 {
			t.Errorf("nested module %q dispatched %d time(s), want exactly 1", nested, dispatch[nested])
		}
	}
}

// TestDiscoverMaintainedGoModulesExcludesFixtures proves fixture and vendored
// modules under testdata/node_modules never enter the maintained-module set.
func TestDiscoverMaintainedGoModulesExcludesFixtures(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, filepath.Join(root, "agent-core"))
	writeGoMod(t, filepath.Join(root, "agent-core", "magefiles"))
	writeGoMod(t, filepath.Join(root, "applications", "catalog", "testdata", "integration", "fixture"))
	writeGoMod(t, filepath.Join(root, "ui", "node_modules", "dep"))

	got, err := discoverMaintainedGoModules(root)
	if err != nil {
		t.Fatalf("discoverMaintainedGoModules: %v", err)
	}
	want := []string{"agent-core", "agent-core/magefiles"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discovered modules = %#v, want %#v", got, want)
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
