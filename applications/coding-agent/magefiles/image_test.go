// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestCodingAgentImageBuildUsesPublishedRecipe(t *testing.T) {
	coreRoot := filepath.Join(string(filepath.Separator), "repo", "agent-core")
	contextDir, dockerfile, args := codingAgentImageBuild(coreRoot, "declarative-agents/agent-core:local", "example/runtime:test")
	if contextDir != coreRoot {
		t.Errorf("build context = %q, want agent-core root", contextDir)
	}
	if dockerfile != filepath.Join(coreRoot, "toolchain.Dockerfile") {
		t.Errorf("Dockerfile = %q", dockerfile)
	}
	want := []string{
		"build", "--pull=false",
		"--build-arg", "GOLANGCI_LINT_VERSION=" + codingAgentGolangciLint,
		"--build-arg", "RUNTIME_IMAGE=declarative-agents/agent-core:local",
		"-f", dockerfile,
		"-t", "example/runtime:test",
		".",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("docker args = %#v, want %#v", args, want)
	}
}

func TestCodingAgentImageBuildHasDedicatedBoundedTimeout(t *testing.T) {
	if codingAgentImageBuildTimeout != 10*time.Minute {
		t.Fatalf("image build timeout = %s, want 10m", codingAgentImageBuildTimeout)
	}
	if codingAgentImageBuildTimeout <= codingHelmClusterTimeout {
		t.Fatalf("image build timeout %s reuses shorter cluster-operation budget %s",
			codingAgentImageBuildTimeout, codingHelmClusterTimeout)
	}
}

func TestChartDefaultsToCodingToolchainImage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "helm", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var values struct {
		Image struct {
			Repository string `yaml:"repository"`
			Tag        string `yaml:"tag"`
		} `yaml:"image"`
	}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if values.Image.Repository != codingAgentImageRepository ||
		values.Image.Tag != codingAgentImageTag {
		t.Fatalf("default image = %s:%s, want %s:%s",
			values.Image.Repository, values.Image.Tag,
			codingAgentImageRepository, codingAgentImageTag)
	}
	if strings.HasSuffix(values.Image.Repository, "/agent-core") {
		t.Fatal("chart reverted to the runtime-only agent-core image")
	}
}
