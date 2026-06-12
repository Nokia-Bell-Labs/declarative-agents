// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

func TestBuiltinRegistry_RegisterAndResolve(t *testing.T) {
	br := NewBuiltinRegistry()
	called := false
	br.Register("test_init", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		called = true
		return &ExecBuilder{Def: def}, nil
	})

	f, ok := br.Resolve("test_init")
	if !ok {
		t.Fatal("expected to resolve test_init")
	}
	td := ToolDef{Name: "test", Type: "builtin", Init: "test_init"}
	if _, err := f(td, nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("factory was not called")
	}
}

func TestBuiltinRegistry_ResolveMissing(t *testing.T) {
	br := NewBuiltinRegistry()
	_, ok := br.Resolve("nonexistent")
	if ok {
		t.Fatal("expected Resolve to return false for missing init")
	}
}

func TestBuiltinRegistry_DuplicatePanics(t *testing.T) {
	br := NewBuiltinRegistry()
	br.Register("dup", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return nil, nil
	})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate register")
		}
	}()
	br.Register("dup", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return nil, nil
	})
}

func TestRegisterUnifiedTools_ExecAndBuiltin(t *testing.T) {
	reg := core.NewRegistry()
	br := NewBuiltinRegistry()

	br.Register("file_read", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &ExecBuilder{Def: ToolDef{Name: def.Name, Binary: "echo"}}, nil
	})

	defs := []ToolDef{
		{Name: "stage_all", Type: "exec", Binary: "git", Args: []string{"add", "-A"}},
		{Name: "read", Type: "builtin", Init: "file_read", Config: map[string]interface{}{"root": "/tmp"}},
	}

	err := RegisterUnifiedTools(reg, br, "/tmp", defs, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := reg.Resolve("stage_all"); !ok {
		t.Error("stage_all not registered")
	}
	if _, ok := reg.Resolve("read"); !ok {
		t.Error("read not registered")
	}
}

func TestRegisterUnifiedTools_BuiltinMissingInit(t *testing.T) {
	reg := core.NewRegistry()
	br := NewBuiltinRegistry()

	defs := []ToolDef{
		{Name: "bad", Type: "builtin"},
	}

	err := RegisterUnifiedTools(reg, br, "/tmp", defs, nil)
	if err == nil {
		t.Fatal("expected error for missing init")
	}
}

func TestRegisterUnifiedTools_UnknownInit(t *testing.T) {
	reg := core.NewRegistry()
	br := NewBuiltinRegistry()

	defs := []ToolDef{
		{Name: "bad", Type: "builtin", Init: "nonexistent"},
	}

	err := RegisterUnifiedTools(reg, br, "/tmp", defs, nil)
	if err == nil {
		t.Fatal("expected error for unknown init")
	}
}

func TestRegisterUnifiedTools_UnknownType(t *testing.T) {
	reg := core.NewRegistry()
	br := NewBuiltinRegistry()

	defs := []ToolDef{
		{Name: "bad", Type: "magic", Binary: "echo"},
	}

	err := RegisterUnifiedTools(reg, br, "/tmp", defs, nil)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestParseToolDefs_BuiltinType(t *testing.T) {
	yaml := `tools:
- name: read
  type: builtin
  init: file_read
  description: "Read a file"
  config:
    root: "/workspace"
`
	defs, err := ParseToolDefs([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	if defs[0].Type != "builtin" {
		t.Errorf("expected type builtin, got %q", defs[0].Type)
	}
	if defs[0].Init != "file_read" {
		t.Errorf("expected init file_read, got %q", defs[0].Init)
	}
	root, ok := defs[0].Config["root"]
	if !ok || root != "/workspace" {
		t.Errorf("expected config root=/workspace, got %v", root)
	}
}

func TestParseToolDefs_BuiltinMissingInit(t *testing.T) {
	yaml := `tools:
- name: bad
  type: builtin
  description: "Missing init"
`
	_, err := ParseToolDefs([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for builtin without init")
	}
}

func TestParseToolDefs_MixedTypes(t *testing.T) {
	yaml := `tools:
- name: stage_all
  binary: git
  args: [add, -A]
  description: "Stage all"
- name: read
  type: builtin
  init: file_read
  description: "Read a file"
`
	defs, err := ParseToolDefs([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}
	if defs[0].Type != "" {
		t.Errorf("expected empty type for exec, got %q", defs[0].Type)
	}
	if defs[1].Type != "builtin" {
		t.Errorf("expected builtin type, got %q", defs[1].Type)
	}
}
