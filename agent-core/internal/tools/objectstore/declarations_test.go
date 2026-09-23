// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// shippedDeclarations resolves tools/builtin/objectstore/all.yaml from this
// source file, so the test exercises the file profiles actually select.
func shippedDeclarations(t *testing.T) []catalog.ToolDef {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller path")
	}
	root := filepath.Join(filepath.Dir(here), "..", "..", "..")
	defs, err := catalog.LoadToolDeclarations([]string{
		filepath.Join(root, "tools", "builtin", "objectstore", "all.yaml"),
	})
	if err != nil {
		t.Fatalf("load shipped declarations: %v", err)
	}
	return defs
}

// srd059 AC1: the shipped file's four words register through the real
// factory and their declared default config resolves to a builder. The
// templated defaults land on mem://agents, so the shipped file validates
// with no environment and no cloud.
func TestShippedDeclarationsRegisterAndResolve(t *testing.T) {
	defs := shippedDeclarations(t)
	if len(defs) != 4 {
		t.Fatalf("shipped declarations carry %d words, want 4", len(defs))
	}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Opener: NewOpener()})
	for _, def := range defs {
		factory, ok := br.Resolve(def.Init)
		if !ok {
			t.Fatalf("init %q has no registered factory", def.Init)
		}
		builder, err := factory(def, nil)
		if err != nil {
			t.Fatalf("resolve %q from the shipped config: %v", def.Name, err)
		}
		if builder == nil {
			t.Fatalf("resolve %q returned no builder", def.Name)
		}
	}
}
