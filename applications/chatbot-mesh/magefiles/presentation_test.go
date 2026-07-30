// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPresentationCommandDisablesPlayground(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "work", "applications", "chatbot-mesh")
	cmd := presentationCommand(root)
	want := []string{"go", "tool", "present", "-play=false", "chatbot-mesh.slide"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("presentation args = %#v, want %#v", cmd.Args, want)
	}
	if cmd.Dir != root {
		t.Fatalf("presentation directory = %s, want %s", cmd.Dir, root)
	}
}
