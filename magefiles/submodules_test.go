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

func TestSubModuleTargets(t *testing.T) {
	targets := []struct {
		name   string
		verb   string
		invoke func([]string, statFunc, func(string) error) error
	}{
		{name: "build", verb: "build", invoke: func(modules []string, stat statFunc, run func(string) error) error {
			return buildSubModules(modules, stat, run)
		}},
		{name: "audit", verb: "audit", invoke: func(modules []string, stat statFunc, run func(string) error) error {
			return auditSubModules(modules, stat, run)
		}},
		{name: "clean", verb: "clean", invoke: func(modules []string, stat statFunc, run func(string) error) error {
			return cleanSubModules(modules, stat, run)
		}},
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			t.Run("runs modules with Magefiles", func(t *testing.T) {
				root := t.TempDir()
				var modules []string
				for _, name := range []string{"agent-core", "agent-profiles", "design-patterns"} {
					module := filepath.Join(root, name)
					mkdir(t, filepath.Join(module, "magefiles"))
					modules = append(modules, module)
				}
				var got []string
				err := target.invoke(modules, os.Stat, func(dir string) error {
					got = append(got, filepath.Base(dir))
					return nil
				})
				if err != nil {
					t.Fatalf("%s submodules returned error: %v", target.verb, err)
				}
				want := []string{"agent-core", "agent-profiles", "design-patterns"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s modules = %#v, want %#v", target.verb, got, want)
				}
			})

			t.Run("skips modules without Magefiles", func(t *testing.T) {
				root := t.TempDir()
				module := filepath.Join(root, "agent-core")
				mkdir(t, filepath.Join(module, "magefiles"))
				without := filepath.Join(root, "no-mage")
				mkdir(t, without)
				var got []string
				err := target.invoke([]string{module, without}, os.Stat, func(dir string) error {
					got = append(got, filepath.Base(dir))
					return nil
				})
				if err != nil {
					t.Fatalf("%s submodules returned error: %v", target.verb, err)
				}
				if !reflect.DeepEqual(got, []string{"agent-core"}) {
					t.Fatalf("%s modules = %#v, want [agent-core]", target.verb, got)
				}
			})

			t.Run("wraps runner error", func(t *testing.T) {
				root := t.TempDir()
				module := filepath.Join(root, "agent-core")
				mkdir(t, filepath.Join(module, "magefiles"))
				want := errors.New(target.verb + " failed")
				err := target.invoke([]string{module}, os.Stat, func(string) error { return want })
				if !errors.Is(err, want) {
					t.Fatalf("%s error = %v, want wrapped %v", target.verb, err, want)
				}
				if !strings.Contains(err.Error(), target.verb+" in "+module) {
					t.Fatalf("%s error = %q, want module context", target.verb, err)
				}
			})
		})
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
