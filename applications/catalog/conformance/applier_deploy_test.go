// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The deploy and undeploy variants run host-side against exec doubles, so
// every case here is offline: no cluster, no Helm, no model provider
// (srd022 R6.2).
const (
	deployMachineRel   = "agents/applier/deploy-machine.yaml"
	undeployMachineRel = "agents/applier/undeploy-machine.yaml"
	overridesContent   = "image:\n  tag: \"conformance\"\n"
)

// deployFixture stages a profile that binds the canonical machine and
// selection to the standalone doubles, appending one fixture unit that
// redeclares the words that must fail. Declaration order is what selects the
// double: a later unit replaces a word the earlier one declared, so a fixture
// names only the word it breaks.
//
// The profile is written to the test's own directory rather than shipped under
// testdata, so no gate that globs profile-shaped files finds a fixture and
// tries to boot it.
func deployFixture(t *testing.T, machineRel, toolsRel, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	declarations := []string{
		ProfilePath("agents/applier/apply-declarations.yaml"),
		ProfilePath("agents/applier/standalone-declarations.yaml"),
	}
	if fixture != "" {
		path := filepath.Join(ProfilesRoot(), "testdata", "conformance", "applier-deploy", fixture, "declarations.yaml")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("fixture %s: %v", fixture, err)
		}
		declarations = append(declarations, path)
	}
	profile := map[string]any{
		"name":              "conformance-applier-" + filepath.Base(filepath.Dir(machineRel)),
		"machine":           ProfilePath(machineRel),
		"tools":             []string{ProfilePath(toolsRel)},
		"tool_declarations": declarations,
	}
	encoded, err := yaml.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// deployRequest writes the seed a host deploy target sends. The runtime turns
// --request into the initial Seed result, and write_overrides reads path and
// content out of its parameters object by name.
func deployRequest(t *testing.T) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"parameters": map[string]string{
			"path":    "overrides.yaml",
			"content": overridesContent,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runDeploy(t *testing.T, machineRel, toolsRel, fixture string) (RunResult, string) {
	t.Helper()
	workspace := t.TempDir()
	return Run(t, RunConfig{
		Profile:   deployFixture(t, machineRel, toolsRel, fixture),
		Directory: workspace,
		Request:   deployRequest(t),
	}), workspace
}

// requireOverridesWritten asserts the decided values reached the workspace file
// the apply words read with -f. The write reports its span under the builtin's
// own name rather than the declared word's, so reading the file back is both
// stronger and less brittle than asserting on a span.
func requireOverridesWritten(t *testing.T, workspace string) {
	t.Helper()
	written, err := os.ReadFile(filepath.Join(workspace, "overrides.yaml"))
	if err != nil {
		t.Fatalf("write_overrides left no values file: %v", err)
	}
	if got := string(written); got != overridesContent {
		t.Errorf("overrides.yaml = %q, want %q", got, overridesContent)
	}
}

// requireNoToolSpan asserts a word never ran. Half of what these cases prove is
// negative: that validation precedes mutation, and that the machine never
// removes a release it cannot show it created.
func requireNoToolSpan(t *testing.T, result RunResult, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if got := result.Spans.Named("execute_tool " + tool); len(got) > 0 {
			t.Errorf("word %q ran and must not have; span names: %v", tool, result.Spans.Names())
		}
	}
}

func TestApplierDeployReachesDeployed(t *testing.T) {
	t.Parallel()
	result, workspace := runDeploy(t, deployMachineRel, "agents/applier/deploy-tools.yaml", "")
	result.RequireTerminalState(t, "Deployed")
	requireOverridesWritten(t, workspace)
	result.RequireToolSpans(t, "helm_dry_run", "helm_upgrade", "verify_rollout")
	requireNoToolSpan(t, result, "helm_rollback")
}

func TestApplierDeployRejectsAFailedDryRun(t *testing.T) {
	t.Parallel()
	result, workspace := runDeploy(t, deployMachineRel, "agents/applier/deploy-tools.yaml", "failed-dry-run")
	result.RequireTerminalState(t, "Rejected")
	requireOverridesWritten(t, workspace)
	result.RequireToolSpans(t, "helm_dry_run")
	requireNoToolSpan(t, result, "helm_upgrade", "verify_rollout", "helm_rollback")
}

func TestApplierDeployRollsBackAStalledVerify(t *testing.T) {
	t.Parallel()
	result, _ := runDeploy(t, deployMachineRel, "agents/applier/deploy-tools.yaml", "stalled-verify")
	result.RequireTerminalState(t, "Compensated")
	result.RequireToolSpans(t, "helm_upgrade", "verify_rollout", "helm_rollback")
}

// The first-install case. There is no prior revision, so the rollback fails and
// the run reaches Failed with the release left standing, rather than removing a
// release it cannot prove it created (srd022 R6.3).
func TestApplierDeployFailsWhenRollbackFails(t *testing.T) {
	t.Parallel()
	result, _ := runDeploy(t, deployMachineRel, "agents/applier/deploy-tools.yaml", "stalled-verify-no-rollback")
	result.RequireTerminalState(t, "Failed")
	result.RequireToolSpans(t, "verify_rollout", "helm_rollback")
}

func TestApplierUndeployRemovesAndReportsAbsent(t *testing.T) {
	t.Parallel()
	present, _ := runDeploy(t, undeployMachineRel, "agents/applier/undeploy-tools.yaml", "")
	present.RequireTerminalState(t, "Removed")
	present.RequireToolSpans(t, "helm_history", "helm_uninstall")

	absent, _ := runDeploy(t, undeployMachineRel, "agents/applier/undeploy-tools.yaml", "absent-release")
	absent.RequireTerminalState(t, "Absent")
	requireNoToolSpan(t, absent, "helm_uninstall")

	// A teardown of nothing is the outcome the caller wanted, so Absent carries
	// a succeeded status and the host target exits zero (srd022 R6.5).
	if _, status := absent.TerminalOutcome(t); status != "succeeded" {
		t.Errorf("Absent status = %q, want succeeded", status)
	}
}

// R6.2: the deploy path is declared transitions alone. The guard looks
// redundant while the selections are short, and it is what fails when a later
// change reaches for a model to decide something.
func TestApplierDeployDeclaresNoModelWord(t *testing.T) {
	t.Parallel()
	modelWords := []string{"invoke_llm", "parse_response", "report_parse_error", "done"}
	for _, name := range []string{"deploy-tools.yaml", "undeploy-tools.yaml"} {
		var selection struct {
			Tools []string `yaml:"tools"`
		}
		unmarshalShipped(t, filepath.Join("agents", "applier", name), &selection)
		if len(selection.Tools) == 0 {
			t.Fatalf("%s selects no words", name)
		}
		for _, selected := range selection.Tools {
			for _, model := range modelWords {
				if selected == model {
					t.Errorf("%s selects the model word %q; the deploy path runs as declared transitions alone", name, model)
				}
			}
		}
	}
}

// Every non-terminal state routes every signal the engine can raise. A state
// that misses one leaves a run with no terminal at all, which is what GH-2294
// cost the rig-doctor.
func TestApplierDeployMachinesRouteEverySignalFromEveryState(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{deployMachineRel, undeployMachineRel} {
		var machine struct {
			InitialState   string   `yaml:"initial_state"`
			TerminalStates []string `yaml:"terminal_states"`
			States         []struct {
				Name string `yaml:"name"`
			} `yaml:"states"`
			Signals []struct {
				Name string `yaml:"name"`
			} `yaml:"signals"`
			Transitions []applierTransition `yaml:"transitions"`
		}
		unmarshalShipped(t, filepath.FromSlash(rel), &machine)

		terminal := map[string]bool{}
		for _, name := range machine.TerminalStates {
			terminal[name] = true
		}
		routed := map[string]bool{}
		for _, transition := range machine.Transitions {
			routed[transition.State+"/"+transition.Signal] = true
		}
		for _, state := range machine.States {
			if terminal[state.Name] {
				continue
			}
			for _, signal := range machine.Signals {
				// The initial state is entered by Seed alone and no word has run
				// in it yet; every other non-terminal state is the reverse.
				wordSignal := signal.Name == "ToolDone" || signal.Name == "ToolFailed"
				if state.Name == machine.InitialState && wordSignal {
					continue
				}
				if signal.Name == "Seed" && state.Name != machine.InitialState {
					continue
				}
				if !routed[state.Name+"/"+signal.Name] {
					t.Errorf("%s: state %s does not route %s", rel, state.Name, signal.Name)
				}
			}
		}
	}
}

// The deploy variants host no listener: a host target seeds them through
// --request, and a profile that opened a port would outlive the run.
func TestApplierDeployProfilesDeclareNoRESTSurface(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"deploy-profile.yaml", "undeploy-profile.yaml"} {
		var profile struct {
			RESTDefinitions []string `yaml:"rest_definitions"`
			Machine         string   `yaml:"machine"`
		}
		unmarshalShipped(t, filepath.Join("agents", "applier", name), &profile)
		if len(profile.RESTDefinitions) != 0 {
			t.Errorf("%s declares rest_definitions %v; the host-side variants host no listener", name, profile.RESTDefinitions)
		}
		if profile.Machine == "" {
			t.Errorf("%s names no machine", name)
		}
	}
}

// The shipped selections must resolve against the shipped declarations, so a
// word renamed in one file and not the other fails here rather than at run time.
func TestApplierDeploySelectionsResolveToDeclaredWords(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for _, unit := range []string{"apply-declarations.yaml", "standalone-declarations.yaml"} {
		var doc struct {
			Tools []struct {
				Name string `yaml:"name"`
			} `yaml:"tools"`
		}
		unmarshalShipped(t, filepath.Join("agents", "applier", unit), &doc)
		for _, tool := range doc.Tools {
			declared[tool.Name] = true
		}
	}
	for _, name := range []string{"deploy-tools.yaml", "undeploy-tools.yaml"} {
		var selection struct {
			Tools []string `yaml:"tools"`
		}
		unmarshalShipped(t, filepath.Join("agents", "applier", name), &selection)
		for _, selected := range selection.Tools {
			if !declared[selected] {
				t.Errorf("%s selects %q, which no shipped declaration unit declares", name, selected)
			}
		}
	}
}
