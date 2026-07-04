// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestUc001AgentArgsIncludesCoreRoot(t *testing.T) {
	profileRoot := filepath.Join("profiles", "agents")
	coreRoot := filepath.Join("repo", "agent-core")
	workDir := filepath.Join("tmp", "workspace")

	got := uc001AgentArgs(profileRoot, coreRoot, workDir)
	want := []string{
		"--profile", filepath.Join(profileRoot, "generator", "profile.yaml"),
		"--directory", workDir,
		"--core-root", coreRoot,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uc001AgentArgs = %#v, want %#v", got, want)
	}
}
