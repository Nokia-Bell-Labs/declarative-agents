// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The fleet observer is one agent the catalog owns and a mesh wraps (GH-2170).
// Its machine, its words, and their selection are the same in every mesh that
// runs an observer; only where it binds, which agents it proxies, and the UI it
// serves differ, and those live in the wrapper's own rest.yaml.

func TestObserverWrapperReferencesCanonicalClosure(t *testing.T) {
	t.Parallel()
	type profile struct {
		Machine          string   `yaml:"machine"`
		Tools            []string `yaml:"tools"`
		ToolDeclarations []string `yaml:"tool_declarations"`
		RESTDefinitions  []string `yaml:"rest_definitions"`
	}
	want := profile{
		Machine: "../../catalog/observer/machine.yaml",
		Tools:   []string{"../../catalog/observer/tools.yaml"},
		ToolDeclarations: []string{
			"../../catalog/observer/declarations.yaml",
			"/opt/agent-core/tools/builtin/lifecycle/exit-agent.yaml",
		},
		RESTDefinitions: []string{"rest.yaml"},
	}
	appsRoot := filepath.Clean(filepath.Join(ProfilesRoot(), ".."))
	path := filepath.Join(appsRoot, "chatbot-mesh", "agents", "observer", "profile.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got profile
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.Machine != want.Machine ||
		strings.Join(got.Tools, ",") != strings.Join(want.Tools, ",") ||
		strings.Join(got.ToolDeclarations, ",") != strings.Join(want.ToolDeclarations, ",") ||
		strings.Join(got.RESTDefinitions, ",") != strings.Join(want.RESTDefinitions, ",") {
		t.Errorf("chatbot-mesh observer does not compose the canonical observer: got %#v want %#v", got, want)
	}
}

func TestObserverWrapperHasNoCopiedCanonicalAssets(t *testing.T) {
	t.Parallel()
	canonicalDir := ProfilePath("agents/observer")
	promoted := []string{"machine.yaml", "tools.yaml", "declarations.yaml"}
	sums := make(map[string][sha256.Size]byte, len(promoted))
	for _, name := range promoted {
		data, err := os.ReadFile(filepath.Join(canonicalDir, name))
		if err != nil {
			t.Fatal(err)
		}
		sums[name] = sha256.Sum256(data)
	}

	appsRoot := filepath.Clean(filepath.Join(ProfilesRoot(), ".."))
	wrapperDir := filepath.Join(appsRoot, "chatbot-mesh", "agents", "observer")
	for name, sum := range sums {
		data, err := os.ReadFile(filepath.Join(wrapperDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if sha256.Sum256(data) == sum {
			t.Errorf("%s is a byte copy of the canonical observer asset; the wrapper must reference it", name)
		} else {
			t.Errorf("%s exists beside the wrapper and has drifted from the canonical observer asset", name)
		}
	}
}
