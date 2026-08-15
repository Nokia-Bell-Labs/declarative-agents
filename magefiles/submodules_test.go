// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSubModuleTargets covers the serial dispatchers whose contract includes
// call order. Audit dispatches concurrently and cannot promise order, so it is
// tested separately in TestAuditSubModules and TestAuditSubModulesLimited.
func TestSubModuleTargets(t *testing.T) {
	targets := []struct {
		name   string
		verb   string
		invoke func([]string, statFunc, func(string) error) error
	}{
		{name: "build", verb: "build", invoke: func(modules []string, stat statFunc, run func(string) error) error {
			return buildSubModules(modules, stat, run)
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
				for _, name := range []string{"agent-core", "applications/catalog", "design-patterns"} {
					module := filepath.Join(root, filepath.FromSlash(name))
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
				want := []string{"agent-core", "catalog", "design-patterns"}
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

// TestAuditSubModules verifies the concurrent audit dispatcher: every runnable
// module runs (order-independent), modules without a mage entrypoint are
// skipped, and a runner failure surfaces wrapped with module context. The
// shared collector is mutex-guarded so the race detector stays quiet.
func TestAuditSubModules(t *testing.T) {
	t.Run("runs every runnable module regardless of order", func(t *testing.T) {
		root := t.TempDir()
		var modules []string
		for _, name := range []string{"agent-core", "applications/catalog", "design-patterns"} {
			module := filepath.Join(root, filepath.FromSlash(name))
			mkdir(t, filepath.Join(module, "magefiles"))
			modules = append(modules, module)
		}
		var mu sync.Mutex
		var got []string
		err := auditSubModules(modules, os.Stat, func(dir string) error {
			mu.Lock()
			got = append(got, filepath.Base(dir))
			mu.Unlock()
			return nil
		})
		if err != nil {
			t.Fatalf("audit submodules returned error: %v", err)
		}
		sort.Strings(got)
		want := []string{"agent-core", "catalog", "design-patterns"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("audit ran modules %#v, want the set %#v", got, want)
		}
	})

	t.Run("skips modules without a mage entrypoint", func(t *testing.T) {
		root := t.TempDir()
		module := filepath.Join(root, "agent-core")
		mkdir(t, filepath.Join(module, "magefiles"))
		without := filepath.Join(root, "no-mage")
		mkdir(t, without)
		var mu sync.Mutex
		var got []string
		err := auditSubModules([]string{module, without}, os.Stat, func(dir string) error {
			mu.Lock()
			got = append(got, filepath.Base(dir))
			mu.Unlock()
			return nil
		})
		if err != nil {
			t.Fatalf("audit submodules returned error: %v", err)
		}
		if !reflect.DeepEqual(got, []string{"agent-core"}) {
			t.Fatalf("audit ran modules %#v, want [agent-core]", got)
		}
	})

	t.Run("wraps a runner error with module context", func(t *testing.T) {
		root := t.TempDir()
		module := filepath.Join(root, "agent-core")
		mkdir(t, filepath.Join(module, "magefiles"))
		want := errors.New("audit failed")
		err := auditSubModules([]string{module}, os.Stat, func(string) error { return want })
		if !errors.Is(err, want) {
			t.Fatalf("audit error = %v, want wrapped %v", err, want)
		}
		if !strings.Contains(err.Error(), "audit in "+module) {
			t.Fatalf("audit error = %q, want module context", err)
		}
	})

	t.Run("surfaces a failure when several modules fail", func(t *testing.T) {
		root := t.TempDir()
		var modules []string
		for _, name := range []string{"a", "b", "c"} {
			module := filepath.Join(root, name)
			mkdir(t, filepath.Join(module, "magefiles"))
			modules = append(modules, module)
		}
		err := auditSubModules(modules, os.Stat, func(string) error {
			return errors.New("boom")
		})
		if err == nil {
			t.Fatal("audit returned nil, want an error from a failing module")
		}
		if !strings.Contains(err.Error(), "audit in "+root) {
			t.Fatalf("audit error = %q, want module context", err)
		}
	})

	t.Run("aborts before dispatch on a non-IsNotExist stat error", func(t *testing.T) {
		want := errors.New("permission denied")
		var ran int32
		err := auditSubModules([]string{"whatever"}, func(string) (os.FileInfo, error) {
			return nil, want
		}, func(string) error {
			atomic.AddInt32(&ran, 1)
			return nil
		})
		if !errors.Is(err, want) {
			t.Fatalf("audit error = %v, want wrapped %v", err, want)
		}
		if got := atomic.LoadInt32(&ran); got != 0 {
			t.Fatalf("runner invoked %d times after a stat error, want 0", got)
		}
	})
}

// TestAuditSubModulesLimited verifies the semaphore never lets more than the
// configured number of module audits run at once.
func TestAuditSubModulesLimited(t *testing.T) {
	const limit = 2
	root := t.TempDir()
	var modules []string
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		module := filepath.Join(root, name)
		mkdir(t, filepath.Join(module, "magefiles"))
		modules = append(modules, module)
	}

	var current, max int32
	err := auditSubModulesLimited(modules, limit, os.Stat, func(string) error {
		now := atomic.AddInt32(&current, 1)
		for {
			peak := atomic.LoadInt32(&max)
			if now <= peak || atomic.CompareAndSwapInt32(&max, peak, now) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		return nil
	})
	if err != nil {
		t.Fatalf("limited audit returned error: %v", err)
	}
	if got := atomic.LoadInt32(&max); got > limit {
		t.Fatalf("peak concurrency = %d, want <= %d", got, limit)
	}
	if got := atomic.LoadInt32(&max); got < 2 {
		t.Fatalf("peak concurrency = %d, expected the dispatch to actually overlap", got)
	}
}

// TestAuditConcurrency guards the default bound against a zero or negative
// NumCPU sentinel: the dispatcher must always run at least one module.
func TestAuditConcurrency(t *testing.T) {
	if got := auditConcurrency(); got < 1 || got > maxConcurrentModuleAudits {
		t.Fatalf("auditConcurrency() = %d, want 1..%d", got, maxConcurrentModuleAudits)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
