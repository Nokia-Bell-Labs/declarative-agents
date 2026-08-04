// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type applierTransition struct {
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action"`
}

func TestCanonicalApplierMachineContracts(t *testing.T) {
	t.Parallel()
	type machine struct {
		TerminalStates []string            `yaml:"terminal_states"`
		Transitions    []applierTransition `yaml:"transitions"`
	}
	var apply machine
	readRoleYAML(t, "agents/applier/apply-machine.yaml", &apply)
	wantTerminals := []string{"Done", "Rejected", "RolledBack", "Failed"}
	if strings.Join(apply.TerminalStates, ",") != strings.Join(wantTerminals, ",") {
		t.Fatalf("apply terminals = %v, want %v", apply.TerminalStates, wantTerminals)
	}
	wantApply := []applierTransition{
		{State: "AwaitingRequest", Signal: "Seed", Next: "Writing", Action: "write_overrides"},
		{State: "Writing", Signal: "ToolDone", Next: "Validating", Action: "helm_dry_run"},
		{State: "Validating", Signal: "ToolDone", Next: "Applying", Action: "helm_upgrade"},
		{State: "Applying", Signal: "ToolDone", Next: "Verifying", Action: "verify_rollout"},
		{State: "Verifying", Signal: "ToolFailed", Next: "RollingBack", Action: "helm_rollback"},
		{State: "RollingBack", Signal: "ToolDone", Next: "RolledBack"},
		{State: "RollingBack", Signal: "ToolFailed", Next: "Failed"},
	}
	for _, want := range wantApply {
		if !containsApplierTransition(apply.Transitions, want) {
			t.Errorf("canonical apply machine misses transition %#v", want)
		}
	}

	var rollout machine
	readRoleYAML(t, "agents/applier/rollout-machine.yaml", &rollout)
	if strings.Join(rollout.TerminalStates, ",") != "Complete,Progressing,Unavailable" {
		t.Errorf("rollout terminals = %v", rollout.TerminalStates)
	}
	for _, want := range []applierTransition{
		{State: "Polling", Signal: "ToolDone", Next: "ReadingComplete", Action: "kubectl_get_rollout_counts"},
		{State: "Polling", Signal: "ToolFailed", Next: "ReadingProgressing", Action: "kubectl_get_rollout_counts"},
		{State: "ReadingProgressing", Signal: "ToolFailed", Next: "Unavailable"},
	} {
		if !containsApplierTransition(rollout.Transitions, want) {
			t.Errorf("canonical rollout machine misses transition %#v", want)
		}
	}
}

func containsApplierTransition(got []applierTransition, want applierTransition) bool {
	for _, item := range got {
		if item == want {
			return true
		}
	}
	return false
}

func TestCanonicalApplierSelectionsAreNameOnly(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"tools.yaml", "apply-tools.yaml", "rollout-tools.yaml"} {
		data, err := os.ReadFile(ProfilePath("agents/applier/" + name))
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Tools []any `yaml:"tools"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if len(doc.Tools) == 0 {
			t.Errorf("%s selects no tools", name)
		}
		for _, item := range doc.Tools {
			if _, ok := item.(string); !ok {
				t.Errorf("%s contains a ToolDef instead of a name: %#v", name, item)
			}
		}
	}
}

func TestApplierPromotionWrappersReferenceCanonicalClosure(t *testing.T) {
	t.Parallel()
	type profile struct {
		Machine          string   `yaml:"machine"`
		Tools            []string `yaml:"tools"`
		ToolDeclarations []string `yaml:"tool_declarations"`
		RESTDefinitions  []string `yaml:"rest_definitions"`
	}
	appsRoot := filepath.Clean(filepath.Join(ProfilesRoot(), ".."))
	for _, app := range []string{"chatbot-mesh", "coding-agent", "agent-architecture"} {
		dir := filepath.Join(appsRoot, app, "agents", "applier")
		cases := map[string]profile{
			"profile.yaml": {
				Machine: "../../catalog/applier/machine.yaml",
				Tools:   []string{"../../catalog/applier/tools.yaml"},
				ToolDeclarations: []string{
					"../../catalog/applier/declarations.yaml",
					"/opt/agent-core/tools/builtin/lifecycle/exit-agent.yaml",
				},
				RESTDefinitions: []string{"rest.yaml"},
			},
			"apply-profile.yaml": {
				Machine: "../../catalog/applier/apply-machine.yaml",
				Tools:   []string{"../../catalog/applier/apply-tools.yaml"},
				ToolDeclarations: []string{
					"../../catalog/applier/apply-declarations.yaml", "exec-declarations.yaml",
				},
				RESTDefinitions: []string{"rest.yaml"},
			},
			"rollout-profile.yaml": {
				Machine:          "../../catalog/applier/rollout-machine.yaml",
				Tools:            []string{"../../catalog/applier/rollout-tools.yaml"},
				ToolDeclarations: []string{"exec-declarations.yaml"},
				RESTDefinitions:  []string{"rest.yaml"},
			},
		}
		for filename, want := range cases {
			var got profile
			data, err := os.ReadFile(filepath.Join(dir, filename))
			if err != nil {
				t.Fatalf("%s %s: %v", app, filename, err)
			}
			if err := yaml.Unmarshal(data, &got); err != nil {
				t.Fatalf("%s %s: %v", app, filename, err)
			}
			if got.Machine != want.Machine ||
				strings.Join(got.Tools, ",") != strings.Join(want.Tools, ",") ||
				strings.Join(got.ToolDeclarations, ",") != strings.Join(want.ToolDeclarations, ",") ||
				strings.Join(got.RESTDefinitions, ",") != strings.Join(want.RESTDefinitions, ",") {
				t.Errorf("%s %s does not compose canonical applier: got %#v want %#v", app, filename, got, want)
			}
		}
	}
}

func TestApplierPromotionHasNoCopiedCanonicalAssets(t *testing.T) {
	t.Parallel()
	canonicalDir := ProfilePath("agents/applier")
	promoted := []string{
		"machine.yaml", "apply-machine.yaml", "rollout-machine.yaml",
		"tools.yaml", "apply-tools.yaml", "rollout-tools.yaml",
		"declarations.yaml", "apply-declarations.yaml",
	}
	type canonicalAsset struct {
		name string
		sum  [sha256.Size]byte
		data []byte
	}
	var assets []canonicalAsset
	for _, name := range promoted {
		data, err := os.ReadFile(filepath.Join(canonicalDir, name))
		if err != nil {
			t.Fatal(err)
		}
		assets = append(assets, canonicalAsset{name: name, sum: sha256.Sum256(data), data: data})
	}

	appsRoot := filepath.Clean(filepath.Join(ProfilesRoot(), ".."))
	var copies []string
	for _, app := range []string{"chatbot-mesh", "coding-agent", "agent-architecture"} {
		root := filepath.Join(appsRoot, app, "agents")
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			for _, asset := range assets {
				if sum == asset.sum && bytes.Equal(data, asset.data) {
					rel, _ := filepath.Rel(appsRoot, path)
					copies = append(copies, fmt.Sprintf("%s copies %s", filepath.ToSlash(rel), asset.name))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(copies)
	if len(copies) != 0 {
		t.Fatalf("application-local copies of promoted applier assets:\n%s", strings.Join(copies, "\n"))
	}
}
