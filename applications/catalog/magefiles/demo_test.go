// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestKnowledgeManagerCommandRunsShippedProfile(t *testing.T) {
	profilesRoot := filepath.Join(string(filepath.Separator), "src", "applications", "catalog")
	coreRoot := filepath.Join(string(filepath.Separator), "src", "agent-core")

	cmd := knowledgeManagerCommand("/tmp/agent", profilesRoot, coreRoot)

	want := []string{
		"/tmp/agent",
		"--profile", filepath.Join(profilesRoot, knowledgeManagerProfile),
		"--directory", profilesRoot,
		"--core-root", coreRoot,
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("cmd.Args = %#v, want %#v", cmd.Args, want)
	}
	if cmd.Dir != profilesRoot {
		t.Fatalf("cmd.Dir = %q, want %q", cmd.Dir, profilesRoot)
	}
}
