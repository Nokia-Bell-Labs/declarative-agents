// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAuditDesignPatternsChecksEvidenceBeforeBuild(t *testing.T) {
	var calls []string

	err := auditDesignPatterns(
		func() error {
			calls = append(calls, "evidence")
			return nil
		},
		func() error {
			calls = append(calls, "build")
			return nil
		},
	)

	if err != nil {
		t.Fatalf("auditDesignPatterns returned error: %v", err)
	}
	if got := strings.Join(calls, ","); got != "evidence,build" {
		t.Fatalf("call order = %q, want evidence,build", got)
	}
}

func TestAuditDesignPatternsStopsOnEvidenceError(t *testing.T) {
	want := errors.New("evidence failed")
	buildCalled := false

	err := auditDesignPatterns(
		func() error { return want },
		func() error {
			buildCalled = true
			return nil
		},
	)

	if !errors.Is(err, want) {
		t.Fatalf("auditDesignPatterns error = %v, want %v", err, want)
	}
	if buildCalled {
		t.Fatal("build ran after evidence failure")
	}
}

func TestAuditDesignPatternsReturnsBuildError(t *testing.T) {
	want := errors.New("build failed")

	err := auditDesignPatterns(func() error { return nil }, func() error { return want })

	if !errors.Is(err, want) {
		t.Fatalf("auditDesignPatterns error = %v, want %v", err, want)
	}
}

func TestReferenceImplementationEvidenceAcceptsShippedChecks(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "agent", "machine.yaml"),
		"name: agent\nstates: [Idle, Done]\ntransitions: []\n")
	language := writePatternLanguage(t, root, `
patterns:
  - id: machine-interpreter
    examples:
      - system: Reference implementation
        cite: reference-implementation
        kind: internal
        note: The shipped machine declares Idle.
        evidence:
          classification: shipped
          checks:
            - path: agent/machine.yaml
              artifact: machine
              assertion: yaml_fields
              fields: [states, transitions]
`)

	if err := auditReferenceImplementationEvidence(language, root); err != nil {
		t.Fatalf("auditReferenceImplementationEvidence: %v", err)
	}
}

func TestReferenceImplementationEvidenceRejectsMissingEvidence(t *testing.T) {
	root := t.TempDir()
	language := writePatternLanguage(t, root, `
patterns:
  - id: machine-interpreter
    examples:
      - system: Unsupported claim
        cite: reference-implementation
        kind: internal
        note: Claims shipped behavior without evidence.
`)

	err := auditReferenceImplementationEvidence(language, root)
	if err == nil || !strings.Contains(err.Error(), "evidence is required") {
		t.Fatalf("error = %v, want missing evidence finding", err)
	}
}

func TestReferenceImplementationEvidenceAcceptsLabeledDesignIntent(t *testing.T) {
	root := t.TempDir()
	language := writePatternLanguage(t, root, `
patterns:
  - id: approval-gate
    examples:
      - system: Design intent
        cite: reference-implementation
        kind: internal
        note: Design intent, not shipped behavior.
        evidence:
          classification: design_intent
`)

	if err := auditReferenceImplementationEvidence(language, root); err != nil {
		t.Fatalf("auditReferenceImplementationEvidence: %v", err)
	}
}

func TestReferenceImplementationEvidenceRejectsUnlabeledFixture(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "fixtures", "machine.yaml"),
		"name: approval\nstates: [Waiting, Done]\ntransitions:\n  - {state: Waiting, signal: Approved, next: Done}\n")
	language := writePatternLanguage(t, root, `
patterns:
  - id: approval-gate
    examples:
      - system: Approval example
        cite: reference-implementation
        kind: internal
        note: An approval profile exists.
        evidence:
          classification: conformance_fixture
          checks:
            - path: fixtures/machine.yaml
              artifact: machine
              assertion: yaml_transition
              match: {state: Waiting, signal: Approved, next: Done}
`)

	err := auditReferenceImplementationEvidence(language, root)
	if err == nil || !strings.Contains(err.Error(), `must say "conformance fixture"`) {
		t.Fatalf("error = %v, want fixture-label finding", err)
	}
}

func TestReferenceImplementationEvidenceRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	language := writePatternLanguage(t, root, `
patterns:
  - id: machine-interpreter
    examples:
      - system: Escaping evidence
        cite: reference-implementation
        kind: internal
        note: A shipped machine exists.
        evidence:
          classification: shipped
          checks:
            - path: ../outside.yaml
              artifact: machine
              assertion: yaml_fields
              fields: [states]
`)

	err := auditReferenceImplementationEvidence(language, root)
	if err == nil || !strings.Contains(err.Error(), "escapes repository root") {
		t.Fatalf("error = %v, want path escape finding", err)
	}
}

func TestReferenceEvidenceRejectsExistingFileOfWrongArtifactType(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "profile.yaml"),
		"name: agent\nmachine: machine.yaml\n")
	check := evidenceCheck{
		Path: "profile.yaml", Artifact: "machine",
		Assertion: "yaml_fields", Fields: []string{"name"},
	}
	err := runEvidenceCheck(root, "wrong type", check)
	if err == nil || !strings.Contains(err.Error(), "requires field \"states\"") {
		t.Fatalf("error = %v, want machine artifact rejection", err)
	}
}

func TestReferenceEvidenceRejectsUnrelatedTokenOutsideGoTestDeclaration(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "claim_test.go"), `package fixture
// TestClaimedBehavior is mentioned but not implemented.
func TestSomethingElse() {}
`)
	check := evidenceCheck{
		Path: "claim_test.go", Artifact: "go_test",
		Assertion: "go_test", Test: "TestClaimedBehavior",
	}
	err := runEvidenceCheck(root, "unrelated token", check)
	if err == nil || !strings.Contains(err.Error(), "is not declared") {
		t.Fatalf("error = %v, want AST test-declaration rejection", err)
	}
}

func TestReferenceEvidenceExecutesFocusedBehaviorTest(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "go.mod"), "module fixture\n\ngo 1.26\n")
	writeAuditFixture(t, filepath.Join(root, "claim_test.go"), `package fixture
import "testing"
func TestClaimedBehavior(t *testing.T) { t.Fatal("behavior regressed") }
`)
	check := evidenceCheck{
		Path: "claim_test.go", Artifact: "go_test",
		Assertion: "go_test", Test: "TestClaimedBehavior",
	}
	err := runEvidenceCheck(root, "failing behavior", check)
	if err == nil || !strings.Contains(err.Error(), "focused Go test") ||
		!strings.Contains(err.Error(), "behavior regressed") {
		t.Fatalf("error = %v, want executed behavior failure", err)
	}
}

func TestReferenceEvidenceRejectsBehaviorallyFalseYAMLRelationship(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "a.yaml"),
		"name: a\nmachine: machine-a.yaml\ntools: [tools.yaml]\ntool_declarations: [llm/a.yaml]\n")
	writeAuditFixture(t, filepath.Join(root, "b.yaml"),
		"name: b\nmachine: machine-b.yaml\ntools: [tools.yaml]\ntool_declarations: [llm/b.yaml]\n")
	check := evidenceCheck{
		Paths: []string{"a.yaml", "b.yaml"}, Artifact: "profile",
		Assertion: "yaml_relation", SameFields: []string{"machine", "tools"},
		DifferentFields: []string{"tool_declarations"},
	}
	err := runEvidenceCheck(root, "false relationship", check)
	if err == nil || !strings.Contains(err.Error(), "machine") {
		t.Fatalf("error = %v, want false same-field rejection", err)
	}
}

func TestReferenceEvidenceValidatesRESTArtifactValues(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "rest.yaml"), `rest:
  servers:
    monitor:
      address: 127.0.0.1:0
      endpoints:
        state: {path: /monitor/state}
`)
	check := evidenceCheck{
		Path: "rest.yaml", Artifact: "rest_definition",
		Assertion: "yaml_value",
		Field:     "rest.servers.monitor.endpoints.state.path",
		Equals:    "/monitor/state",
	}
	if err := runEvidenceCheck(root, "REST route", check); err != nil {
		t.Fatal(err)
	}
	check.Equals = "/read_state"
	if err := runEvidenceCheck(root, "REST route", check); err == nil {
		t.Fatal("false REST route value accepted")
	}
}

func TestProfileRelationshipEvidenceRejectsMachineAndDeclarationArtifacts(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "profile.yaml"),
		"name: executor\nmachine: machine.yaml\ntools: [tools.yaml]\n")
	writeAuditFixture(t, filepath.Join(root, "machine.yaml"),
		"name: executor\nstates: [Idle]\ntransitions: []\n")
	writeAuditFixture(t, filepath.Join(root, "declaration.yaml"),
		"tools:\n  - {name: invoke_llm, type: builtin}\n")

	for _, wrongPath := range []string{"machine.yaml", "declaration.yaml"} {
		check := evidenceCheck{
			Paths: []string{"profile.yaml", wrongPath}, Artifact: "profile",
			Assertion: "yaml_relation", SameFields: []string{"machine"},
		}
		err := runEvidenceCheck(root, "profile grid", check)
		if err == nil || !strings.Contains(err.Error(), "artifact type profile") {
			t.Errorf("%s error = %v, want non-profile rejection", wrongPath, err)
		}
	}
}

func TestRepositoryReferenceImplementationEvidence(t *testing.T) {
	if err := auditReferenceImplementationEvidence("../pattern-language.yaml", "../.."); err != nil {
		t.Fatalf("repository evidence audit: %v", err)
	}
}

func TestApprovalGateChapterUsesCurrentCLIAndLabelsScope(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "10-approval-gate.md"))
	if err != nil {
		t.Fatal(err)
	}
	chapter := string(data)
	for _, required := range []string{
		"bin/agent --profile",
		"--resume-checkpoint \"$RUN_ID\"",
		"--resume-signal Approved",
		"--resume-signal Rejected",
		"conformance fixture",
		"design intent",
		"no rollback runs",
	} {
		if !strings.Contains(chapter, required) {
			t.Errorf("Approval Gate chapter missing %q", required)
		}
	}
	for _, stale := range []string{"agent resume", "--checkpoint <id>", "--reason "} {
		if strings.Contains(chapter, stale) {
			t.Errorf("Approval Gate chapter retains unsupported CLI %q", stale)
		}
	}
}

func TestOperatorPortChapterUsesShippedRoutesAndDiscovery(t *testing.T) {
	chapterData, err := os.ReadFile(filepath.Join("..", "12-operator-port.md"))
	if err != nil {
		t.Fatal(err)
	}
	chapter := string(chapterData)
	for _, required := range []string{
		"/monitor/machine",
		"/monitor/state",
		"/monitor/tools",
		"/monitor/metrics",
		"/monitor/events",
		"/monitor/events/stream",
		"/monitor/openapi",
		"/monitor/control/exit",
		"REST launch output",
		"conformance",
		"design intent",
		"PID/profile discovery file",
	} {
		if !strings.Contains(chapter, required) {
			t.Errorf("Operator Port chapter missing %q", required)
		}
	}
	for _, stale := range []string{
		"`/read_state`", "`/stream_events`", "`/emit_signal`", "`/lifecycle_control`",
	} {
		if strings.Contains(chapter, stale) {
			t.Errorf("Operator Port chapter retains invented endpoint %s", stale)
		}
	}
	srd, err := os.ReadFile(filepath.Join("..", "..", "agent-core", "docs", "specs",
		"software-requirements", "srd033-monitor-rest-api.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(srd), "discover the bound address from the REST server launch output") {
		t.Fatal("monitor SRD no longer defines launch-output address discovery")
	}
}

func TestBidirectionalLogChapterKeepsCodingScenariosAsDesignIntent(t *testing.T) {
	chapterData, err := os.ReadFile(filepath.Join("..", "07-bidirectional-log.md"))
	if err != nil {
		t.Fatal(err)
	}
	chapter := string(chapterData)
	intentIndex := strings.Index(chapter, "## Design intent scenarios")
	knownIndex := strings.Index(chapter, "## Known Uses")
	if intentIndex < 0 || knownIndex < 0 || intentIndex >= knownIndex {
		t.Fatal("Bidirectional Log chapter must place design intent before Known Uses")
	}
	knownUses := chapter[knownIndex:]
	for _, forbidden := range []string{
		"**Coding agents.**", "**Gated deployment.**", "**Multi-step API plans.**",
	} {
		if strings.Contains(knownUses, forbidden) {
			t.Errorf("Known Uses retains unshipped scenario %s", forbidden)
		}
	}
	for _, required := range []string{
		"No production coding-agent profile routes validation failure",
		"**Reference rollback mechanics.**",
		"**Database transactions and rollback**",
		"**Memento pattern**",
		"**Event Sourcing**",
	} {
		if !strings.Contains(chapter, required) {
			t.Errorf("Bidirectional Log chapter missing %q", required)
		}
	}

	language, err := os.ReadFile(filepath.Join("..", "pattern-language.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Reference implementation — checkpoint and receipt rollback mechanics",
		"Design intent — coding-agent rollback",
		"no production profile routes validation failure into checkpoint_rollback",
	} {
		if !strings.Contains(string(language), required) {
			t.Errorf("pattern language missing %q", required)
		}
	}

	codingAgents := filepath.Join("..", "..", "examples", "coding-agent", "agents")
	err = filepath.WalkDir(codingAgents, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || filepath.Ext(path) != ".yaml" {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "checkpoint_rollback") {
			t.Errorf("production coding-agent profile wires checkpoint_rollback in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAgentAsDataUsesCanonicalFamiliesAndLabelsConceptualRoles(t *testing.T) {
	chapterData, err := os.ReadFile(filepath.Join("..", "03-agent-as-data.md"))
	if err != nil {
		t.Fatal(err)
	}
	chapter := string(chapterData)
	for _, stale := range []string{
		"(`generator`)", "(`evaluator`)", "generator and evaluator profiles",
		"Generator profiles", "bench/evaluator",
	} {
		if strings.Contains(chapter, stale) {
			t.Errorf("Agent-as-Data retains current-family term %q", stale)
		}
	}
	for _, required := range []string{
		"`executor`, `critic`, `bench`, and `jurist`",
		"executor and critic profiles",
		"generator* and *evaluator* describe conceptual roles",
	} {
		if !strings.Contains(chapter, required) {
			t.Errorf("Agent-as-Data missing %q", required)
		}
	}
	figure, err := os.ReadFile(filepath.Join("..", "figures", "fig-09-profile-packages.puml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{"generator/", "evaluator/"} {
		if strings.Contains(string(figure), stale) {
			t.Errorf("profile package figure retains %q", stale)
		}
	}

	languageData, err := os.ReadFile(filepath.Join("..", "pattern-language.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var language patternLanguageEvidence
	if err := yaml.Unmarshal(languageData, &language); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range language.Patterns {
		for _, example := range pattern.Examples {
			if example.Cite != referenceImplementationCitation || example.Kind != "internal" {
				continue
			}
			currentClaim := strings.ToLower(example.System + " " + example.Note)
			for _, retired := range []string{"generator", "evaluator"} {
				if strings.Contains(currentClaim, retired) {
					t.Errorf("%s internal claim uses retired family %q",
						pattern.ID, retired)
				}
			}
		}
	}

	introduction, err := os.ReadFile(filepath.Join("..", "01-introduction.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(introduction), "example of a code **generator** agent") {
		t.Fatal("conceptual generator example was removed instead of labelled")
	}
}

func TestInferenceBoundaryUsesLoadableProfilesAndSeparatesProviderChanges(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "06-inference-boundary.md"))
	if err != nil {
		t.Fatal(err)
	}
	chapter := string(data)
	for _, required := range []string{
		"`profile.yaml`, `profile-qwen35b.yaml`, and `profile-qwen27b.yaml`",
		"`qwen3.6:35b-mlx`",
		"`qwen3.6:27b-mlx`",
		"Ollama is the only shipped provider adapter",
		"requires a new adapter",
	} {
		if !strings.Contains(chapter, required) {
			t.Errorf("Inference Boundary missing %q", required)
		}
	}
	for _, stale := range []string{
		"`deepseek.yaml`", "`devstral.yaml`",
		"switching providers is a configuration change",
		"one-line provider portability",
	} {
		if strings.Contains(chapter, stale) {
			t.Errorf("Inference Boundary retains false claim %q", stale)
		}
	}
}

func TestPhaseScopedToolsetUsesDerivedAvailabilityModel(t *testing.T) {
	chapterData, err := os.ReadFile(filepath.Join("..", "05-phase-scoped-toolset.md"))
	if err != nil {
		t.Fatal(err)
	}
	chapter := string(chapterData)
	for _, required := range []string{
		"`$tool` transitions",
		"emitted signals",
		"tool-level phase",
		"`ApplyDynamicToolPhases`",
		"`ResolveExternalTool`/`AvailableIn`",
		"phase-scoped-machine-example",
		"phase-scoped-tools-example",
	} {
		if !strings.Contains(chapter, required) {
			t.Errorf("Phase-Scoped Toolset missing %q", required)
		}
	}
	for _, stale := range []string{
		"per-state tool list",
		"each state carries an optional `tools` field",
		"Every state carries a tool list",
		"absent field means \"all external tools\"",
	} {
		if strings.Contains(chapter, stale) {
			t.Errorf("Phase-Scoped Toolset retains stale model %q", stale)
		}
	}
	for _, figureName := range []string{
		"fig-16-scoped-toolset-class.puml",
		"fig-17-scoped-toolset-sequence.puml",
	} {
		figure, err := os.ReadFile(filepath.Join("..", "figures", figureName))
		if err != nil {
			t.Fatal(err)
		}
		for _, stale := range []string{"per-state tool lists", "visible_tools(state)"} {
			if strings.Contains(string(figure), stale) {
				t.Errorf("%s retains stale model %q", figureName, stale)
			}
		}
	}

	format, err := os.ReadFile(filepath.Join("..", "..", "agent-core", "docs",
		"specs", "config-formats", "machine-format.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"MachineSpec transitions are the source of truth",
		"Explicit ToolSpec phases may further narrow availability",
		"they are not a second workflow definition",
	} {
		if !strings.Contains(string(format), required) {
			t.Errorf("canonical machine format missing %q", required)
		}
	}
}

func writePatternLanguage(t *testing.T, root, content string) string {
	t.Helper()
	path := filepath.Join(root, "pattern-language.yaml")
	writeAuditFixture(t, path, content)
	return path
}

func writeAuditFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
