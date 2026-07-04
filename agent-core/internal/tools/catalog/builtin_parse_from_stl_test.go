// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import "testing"

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
