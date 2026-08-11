// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
	"gopkg.in/yaml.v3"
)

type tracerProfile struct {
	Name             string   `yaml:"name"`
	Machine          string   `yaml:"machine"`
	Tools            []string `yaml:"tools"`
	ToolDeclarations []string `yaml:"tool_declarations"`
	RestDefinitions  []string `yaml:"rest_definitions"`
}

type tracerMachine struct {
	Name           string   `yaml:"name"`
	InitialState   string   `yaml:"initial_state"`
	States         []any    `yaml:"states"`
	TerminalStates []string `yaml:"terminal_states"`
	Signals        []any    `yaml:"signals"`
	Transitions    []struct {
		State   string `yaml:"state"`
		Signal  string `yaml:"signal"`
		Next    string `yaml:"next"`
		Action  string `yaml:"action"`
		Label   string `yaml:"label"`
		Summary bool   `yaml:"summary"`
	} `yaml:"transitions"`
}

type toolSelection struct {
	Tools []string `yaml:"tools"`
}

type declarationFile struct {
	Tools []struct {
		Name          string         `yaml:"name"`
		Type          string         `yaml:"type"`
		Init          string         `yaml:"init"`
		Args          []string       `yaml:"args"`
		Config        map[string]any `yaml:"config"`
		Reversibility struct {
			Classification string `yaml:"classification"`
		} `yaml:"reversibility"`
		Undo struct {
			Strategy string   `yaml:"strategy"`
			Requires []string `yaml:"requires"`
		} `yaml:"undo"`
		SideEffects []struct {
			Kind   string `yaml:"kind"`
			Target string `yaml:"target"`
			State  string `yaml:"state"`
		} `yaml:"side_effects"`
	} `yaml:"tools"`
}

func TestChildRequestPolicyAndPathsAreDeclared(t *testing.T) {
	root := realApplicationRoot(t)
	var declarations declarationFile
	readTestYAML(t, filepath.Join(
		root, "agents/workflow-orchestrator/declarations.yaml",
	), &declarations)
	byName := make(map[string]struct {
		args   []string
		config map[string]any
	}, len(declarations.Tools))
	for _, tool := range declarations.Tools {
		byName[tool.Name] = struct {
			args   []string
			config map[string]any
		}{args: tool.Args, config: tool.Config}
	}
	first := byName["compose_structure_request"].config["template"]
	retry := byName["compose_structure_retry_request"].config["template"]
	if !strings.Contains(first.(string), "Improve structure without changing claims.") {
		t.Fatalf("first structure intent is not declared: %v", first)
	}
	if !strings.Contains(retry.(string), "Apply critic feedback while preserving immutable claims.") {
		t.Fatalf("retry structure intent is not declared: %v", retry)
	}
	paths := map[string]string{
		"persist_structure_request":       ".tracer/requests/structure-1.json",
		"persist_critic_request":          ".tracer/requests/critic-1.json",
		"persist_structure_retry_request": ".tracer/requests/structure-2.json",
		"persist_critic_retry_request":    ".tracer/requests/critic-2.json",
	}
	for name, path := range paths {
		args := byName[name].args
		if len(args) < 2 || args[1] != path {
			t.Errorf("%s path args = %v, want %s", name, args, path)
		}
	}
}

func TestManifestRevisionWordsDeclareEventAndTerminal(t *testing.T) {
	root := realApplicationRoot(t)
	var declarations declarationFile
	readTestYAML(t, filepath.Join(
		root, "agents/workflow-orchestrator/declarations.yaml",
	), &declarations)
	expected := map[string][]string{
		"append_capture_manifest":         {"append-manifest-revision", "capture_manifested", "none", "1", "0"},
		"append_structure_manifest":       {"append-manifest-revision", "structure_manifested", "none", "2", "1"},
		"append_critique_manifest":        {"append-manifest-revision", "critique_manifested", "none", "3", "0"},
		"append_retry_manifest":           {"append-manifest-revision", "retry_recorded", "none", "4", "0"},
		"append_structure_retry_manifest": {"append-manifest-revision", "structure_retry_manifested", "none", "5", "2"},
		"append_critique_retry_manifest":  {"append-manifest-revision", "critique_retry_manifested", "none", "6", "0"},
		"append_kept_original_manifest":   {"append-manifest-revision", "kept_original", "KeptOriginal", "7", "0"},
		"append_final_manifest":           {"append-manifest-revision", "locally_finalized", "LocallyFinalized", "4", "0"},
		"append_final_retry_manifest":     {"append-manifest-revision", "locally_finalized", "LocallyFinalized", "7", "0"},
	}
	for _, tool := range declarations.Tools {
		if want, ok := expected[tool.Name]; ok {
			if !reflect.DeepEqual(tool.Args, want) {
				t.Errorf("%s args = %v, want %v", tool.Name, tool.Args, want)
			}
			delete(expected, tool.Name)
		}
	}
	if len(expected) != 0 {
		t.Fatalf("missing manifest revision declarations: %v", expected)
	}
}

func TestChildResponseTransitionsDeclareRunSummary(t *testing.T) {
	root := realApplicationRoot(t)
	cases := map[string]map[string]bool{
		"specialist-editor": {"compose_structure_response": true},
		"voice-critic": {
			"compose_critic_pass_response":   true,
			"compose_critic_reject_response": true,
		},
	}
	for agent, expected := range cases {
		var machine tracerMachine
		readTestYAML(t, filepath.Join(root, "agents", agent, "machine.yaml"), &machine)
		for _, transition := range machine.Transitions {
			if expected[transition.Action] && !transition.Summary {
				t.Errorf("%s action %s does not declare summary", agent, transition.Action)
			}
		}
	}
}

func TestSpecialistGroundingOutcomeIsSuccessfulDomainTerminal(t *testing.T) {
	root := realApplicationRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "agents/specialist-editor/machine.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var machine struct {
		States []yaml.Node `yaml:"states"`
	}
	if err := yaml.Unmarshal(data, &machine); err != nil {
		t.Fatal(err)
	}
	statuses := make(map[string]string)
	for _, node := range machine.States {
		var state struct {
			Name      string `yaml:"name"`
			RunStatus string `yaml:"run_status"`
		}
		if node.Kind == yaml.MappingNode {
			if err := node.Decode(&state); err != nil {
				t.Fatal(err)
			}
			statuses[state.Name] = state.RunStatus
		}
	}
	if statuses["InsufficientGrounding"] != "succeeded" {
		t.Errorf("InsufficientGrounding run_status = %q, want succeeded",
			statuses["InsufficientGrounding"])
	}
	if statuses["Failed"] != "failed" {
		t.Errorf("Failed run_status = %q, want failed", statuses["Failed"])
	}
}

func TestSelfInvokeWordsDeclareCompensationContract(t *testing.T) {
	root := realApplicationRoot(t)
	var declarations declarationFile
	readTestYAML(t, filepath.Join(
		root, "agents/workflow-orchestrator/declarations.yaml",
	), &declarations)
	for _, tool := range declarations.Tools {
		if tool.Init != "self_invoke" {
			continue
		}
		if tool.Reversibility.Classification != "compensatable" {
			t.Errorf("%s classification = %q", tool.Name, tool.Reversibility.Classification)
		}
		if tool.Undo.Strategy != "child_agent_workspace_restore" {
			t.Errorf("%s undo strategy = %q", tool.Name, tool.Undo.Strategy)
		}
		want := []string{"child_workspace_ref", "child_trace"}
		if !reflect.DeepEqual(tool.Undo.Requires, want) {
			t.Errorf("%s undo requires = %v, want %v", tool.Name, tool.Undo.Requires, want)
		}
	}
}

func TestTracerBoundaryWordsDeclareEveryTouchedArtifact(t *testing.T) {
	root := realApplicationRoot(t)
	var declarations declarationFile
	readTestYAML(t, filepath.Join(
		root, "agents/workflow-orchestrator/declarations.yaml",
	), &declarations)
	for _, tool := range declarations.Tools {
		expected, tracked := tracerBoundaryTargets[tool.Name]
		if !tracked {
			continue
		}
		var actual []string
		for _, effect := range tool.SideEffects {
			actual = append(actual, effect.Target)
			if strings.HasPrefix(effect.Target, "workproducts/") {
				t.Errorf("%s retains stale target %q", tool.Name, effect.Target)
			}
		}
		sort.Strings(actual)
		sort.Strings(expected)
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s side-effect targets = %v, want %v", tool.Name, actual, expected)
		}
	}
}

var appendManifestTargets = []string{
	"manifest-history", "manifest.json", "manifest artifact selection", "boundary-receipts.jsonl",
}

var tracerBoundaryTargets = map[string][]string{
	"capture_source": {
		"PROSE_TRACER_FIXTURES source fixture", ".tracer/captured-source.md",
		"manifest.json", "boundary-receipts.jsonl",
	},
	"write_original": {
		"00-original.md", "manifest.json", "boundary-receipts.jsonl",
	},
	"append_capture_manifest":         appendManifestTargets,
	"append_structure_manifest":       appendManifestTargets,
	"append_critique_manifest":        appendManifestTargets,
	"append_retry_manifest":           appendManifestTargets,
	"append_structure_retry_manifest": appendManifestTargets,
	"append_critique_retry_manifest":  appendManifestTargets,
	"append_kept_original_manifest":   appendManifestTargets,
	"append_final_manifest":           appendManifestTargets,
	"append_final_retry_manifest":     appendManifestTargets,
	"persist_structure_request": {
		".tracer/requests/structure-1.json", "boundary-receipts.jsonl",
	},
	"persist_critic_request": {
		".tracer/requests/critic-1.json", "boundary-receipts.jsonl",
	},
	"persist_structure_retry_request": {
		".tracer/requests/structure-2.json", "boundary-receipts.jsonl",
	},
	"persist_critic_retry_request": {
		".tracer/requests/critic-2.json", "boundary-receipts.jsonl",
	},
	"write_structure_attempt": {
		"attempts/structure", "manifest.json", "boundary-receipts.jsonl",
	},
	"write_critique_attempt": {
		"attempts/critique", "manifest.last_critic", "manifest.json",
		"boundary-receipts.jsonl",
	},
	"materialize_final_chain": {
		"10-structure.md", "40-critique.json", "final.md",
		"manifest.json", "boundary-receipts.jsonl",
	},
}

func TestTracerManifestClosureIsPortableAndExact(t *testing.T) {
	root := realApplicationRoot(t)
	manifest, err := loadApplicationManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateTracerManifest(manifest); err != nil {
		t.Fatal(err)
	}
	config, err := loadDemoConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	catalogRoot, err := resolveCatalogRoot(root, config)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := appmanifest.Resolve(manifest, appmanifest.Options{
		ApplicationRoot: root,
		CatalogRoot:     catalogRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Roots) != 5 {
		t.Fatalf("closure roots = %d, want 5", len(inventory.Roots))
	}
	for _, file := range inventory.Files {
		for _, value := range []string{file.Source, file.RuntimePath} {
			if path.IsAbs(value) || strings.Contains(value, `\`) ||
				value == ".." || strings.HasPrefix(value, "../") {
				t.Errorf("non-portable closure path %q", value)
			}
		}
	}
}

func TestTracerProfilesUseNameOnlyToolsAndCloseActions(t *testing.T) {
	root := realApplicationRoot(t)
	for _, relative := range tracerProfiles[:3] {
		profilePath := filepath.Join(root, filepath.FromSlash(relative))
		var profile tracerProfile
		readTestYAML(t, profilePath, &profile)
		base := filepath.Dir(profilePath)
		if len(profile.Tools) != 1 || len(profile.ToolDeclarations) != 1 {
			t.Fatalf("%s profile references = %#v", relative, profile)
		}

		var selectionNode yaml.Node
		readTestYAML(t, filepath.Join(base, profile.Tools[0]), &selectionNode)
		assertNameOnlyTools(t, relative, &selectionNode)

		var selection toolSelection
		readTestYAML(t, filepath.Join(base, profile.Tools[0]), &selection)
		var declarations declarationFile
		readTestYAML(t, filepath.Join(base, profile.ToolDeclarations[0]), &declarations)
		declared := make(map[string]bool, len(declarations.Tools))
		for _, tool := range declarations.Tools {
			declared[tool.Name] = true
		}
		sort.Strings(selection.Tools)
		var names []string
		for name := range declared {
			names = append(names, name)
		}
		sort.Strings(names)
		if !reflect.DeepEqual(selection.Tools, names) {
			t.Fatalf("%s selected tools = %v, declarations = %v", relative, selection.Tools, names)
		}

		var machine tracerMachine
		readTestYAML(t, filepath.Join(base, profile.Machine), &machine)
		used := map[string]bool{}
		for _, transition := range machine.Transitions {
			if transition.Action == "" {
				continue
			}
			if !declared[transition.Action] {
				t.Errorf("%s action %q has no owned ToolDef", relative, transition.Action)
			}
			used[transition.Action] = true
		}
		for _, selected := range selection.Tools {
			if !used[selected] {
				t.Errorf("%s selects unused tool %q", relative, selected)
			}
		}
	}
}

func TestOrchestratorGraphHasOneBoundedStructureReplay(t *testing.T) {
	root := realApplicationRoot(t)
	var machine tracerMachine
	readTestYAML(t, filepath.Join(root, "agents/workflow-orchestrator/machine.yaml"), &machine)
	sort.Strings(machine.TerminalStates)
	if want := []string{"Failed", "KeptOriginal", "LocallyFinalized"}; !reflect.DeepEqual(machine.TerminalStates, want) {
		t.Fatalf("orchestrator terminals = %v, want %v", machine.TerminalStates, want)
	}

	assertTransition(t, machine, "RoutingCritic", "CriticRejected", "ManifestingRetry")
	assertTransition(t, machine, "RoutingCriticRetry", "CriticRejected", "ManifestingKeptOriginal")
	assertTransition(t, machine, "RoutingCritic", "CriticAccepted", "MaterializingFinalFirst")
	assertTransition(t, machine, "RoutingCriticRetry", "CriticAccepted", "MaterializingFinalRetry")

	retryInvocations := 0
	criticInvocations := 0
	for _, transition := range machine.Transitions {
		if transition.Action == "invoke_structure_editor_retry" {
			retryInvocations++
		}
		if transition.Action == "invoke_voice_critic" || transition.Action == "invoke_voice_critic_retry" {
			criticInvocations++
		}
		if strings.HasPrefix(transition.Next, "MaterializingFinal") && transition.Signal != "CriticAccepted" {
			t.Errorf("finalization has non-acceptance incoming edge: %#v", transition)
		}
		for _, forbidden := range []string{
			"invoke_voice_editor", "invoke_style_editor", "pangram", "git_", "gh_",
			"helm", "kubectl", "applier",
		} {
			if strings.Contains(strings.ToLower(transition.Action), forbidden) {
				t.Errorf("orchestrator has out-of-scope action %q", transition.Action)
			}
		}
	}
	if retryInvocations != 1 {
		t.Fatalf("structure retry invocations = %d, want 1", retryInvocations)
	}
	if criticInvocations != 2 {
		t.Fatalf("critic invocations = %d, want initial plus one retry", criticInvocations)
	}
}

func TestAuthorityBoundariesAndCriticIndependence(t *testing.T) {
	root := realApplicationRoot(t)
	actors := []string{"workflow-orchestrator", "specialist-editor", "voice-critic"}
	for _, actor := range actors {
		var declarations declarationFile
		readTestYAML(t, filepath.Join(root, "agents", actor, "declarations.yaml"), &declarations)
		for _, tool := range declarations.Tools {
			for _, effect := range tool.SideEffects {
				if actor != "workflow-orchestrator" && effect.Kind == "filesystem_write" {
					t.Errorf("%s tool %s has workproduct mutation authority", actor, tool.Name)
				}
			}
		}
	}

	criticFiles := []string{"profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml"}
	for _, name := range criticFiles {
		data, err := os.ReadFile(filepath.Join(root, "agents", "voice-critic", name))
		if err != nil {
			t.Fatal(err)
		}
		content := strings.ToLower(string(data))
		for _, forbidden := range []string{"pangram", "github", "git_", "gh_", "filesystem_write"} {
			if strings.Contains(content, forbidden) {
				t.Errorf("voice-critic %s contains forbidden Release 00.1 authority %q", name, forbidden)
			}
		}
		for _, leaked := range []string{"editor_reasoning", "editor_prompt", "verdict_recommendation"} {
			if strings.Contains(content, leaked) {
				t.Errorf("voice-critic %s admits editor state %q", name, leaked)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "agents", "voice-critic", "declarations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range []string{
		"semantic_preservation", "structural_intent", "voice_match",
		"tightening_quality", "unsupported_additions", "anchor_copy_risk",
	} {
		if !strings.Contains(string(data), category) {
			t.Errorf("critic schema missing finding category %s", category)
		}
	}
}

func TestStructureEditorAndRAGWrapperConformance(t *testing.T) {
	root := realApplicationRoot(t)
	var machine tracerMachine
	readTestYAML(t, filepath.Join(root, "agents/specialist-editor/machine.yaml"), &machine)
	assertTransition(t, machine, "QueryingStructure", "StructureEvidenceInsufficient", "RefiningQuery")
	assertTransition(t, machine, "QueryingRefinedStructure", "StructureEvidenceInsufficient", "InsufficientGrounding")
	assertTransition(t, machine, "QueryingStructure", "StructureEvidenceReady", "ComposingEditContext")
	assertTransition(t, machine, "QueryingRefinedStructure", "StructureEvidenceReady", "ComposingEditContext")

	var wrapper tracerProfile
	readTestYAML(t, filepath.Join(root, "agents/structure-rag/profile.yaml"), &wrapper)
	for _, ref := range append(append([]string{wrapper.Machine}, wrapper.Tools...), wrapper.ToolDeclarations...) {
		if !strings.Contains(ref, "catalog/knowledge-manager/corpus-reader/") {
			t.Errorf("structure-rag reference is not canonical corpus-reader: %q", ref)
		}
		if strings.Contains(ref, "chatbot-mesh") {
			t.Errorf("structure-rag has cross-application dependency: %q", ref)
		}
	}
	rest, err := os.ReadFile(filepath.Join(root, "agents/structure-rag/rest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	content := strings.ToLower(string(rest))
	for _, forbidden := range []string{"add_records", "delete_records", "/add", "/delete"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("structure-rag REST closure contains collection mutation %q", forbidden)
		}
	}
}

func TestProseEditorRoleBindingsResolve(t *testing.T) {
	root := realApplicationRoot(t)
	roleModel := filepath.Join(root, "..", "docs", "specs", "semantic-models", "agent-role-realizations.yaml")
	var model struct {
		Bindings map[string][]struct {
			Actor          string   `yaml:"actor"`
			PrimaryRole    string   `yaml:"primary_role"`
			SubRoles       []string `yaml:"sub_roles"`
			Inherits       string   `yaml:"inherits"`
			Profile        string   `yaml:"profile"`
			Classification string   `yaml:"classification"`
		} `yaml:"bindings"`
	}
	readTestYAML(t, roleModel, &model)
	got := model.Bindings["prose_editor"]
	if len(got) != 4 {
		t.Fatalf("Prose Editor role bindings = %d, want 4", len(got))
	}
	byActor := map[string]struct {
		role string
		sub  []string
	}{}
	for _, binding := range got {
		if binding.Classification == "wrapper" {
			if binding.Actor != "structure-rag" || binding.Inherits != "corpus-reader" {
				t.Errorf("invalid wrapper binding: %#v", binding)
			}
			continue
		}
		byActor[binding.Actor] = struct {
			role string
			sub  []string
		}{binding.PrimaryRole, binding.SubRoles}
	}
	if byActor["workflow-orchestrator"].role != "executor" ||
		!reflect.DeepEqual(byActor["workflow-orchestrator"].sub, []string{"workflow-orchestrator"}) {
		t.Errorf("workflow-orchestrator binding = %#v", byActor["workflow-orchestrator"])
	}
	if byActor["specialist-editor"].role != "executor" ||
		!reflect.DeepEqual(byActor["specialist-editor"].sub, []string{"change-manager"}) {
		t.Errorf("specialist-editor binding = %#v", byActor["specialist-editor"])
	}
	if byActor["voice-critic"].role != "critic" {
		t.Errorf("voice-critic binding = %#v", byActor["voice-critic"])
	}
}

func TestTracerProfilesBootThroughAgentCore(t *testing.T) {
	if err := validateTracerProfileBoot(realApplicationRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func readTestYAML(t *testing.T, filename string, out any) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
}

func assertNameOnlyTools(t *testing.T, profile string, document *yaml.Node) {
	t.Helper()
	if len(document.Content) == 0 {
		t.Fatalf("%s tools document is empty", profile)
	}
	root := document.Content[0]
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value != "tools" {
			continue
		}
		for _, item := range root.Content[index+1].Content {
			if item.Kind != yaml.ScalarNode {
				t.Fatalf("%s tools.yaml contains ToolDef instead of name-only selection", profile)
			}
		}
		return
	}
	t.Fatalf("%s tools.yaml has no tools selection", profile)
}

func assertTransition(t *testing.T, machine tracerMachine, state, signal, next string) {
	t.Helper()
	for _, transition := range machine.Transitions {
		if transition.State == state && transition.Signal == signal && transition.Next == next {
			return
		}
	}
	t.Errorf("missing transition %s + %s -> %s", state, signal, next)
}
