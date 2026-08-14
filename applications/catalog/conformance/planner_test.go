// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
)

// plannerModel is the model the planner LLM declaration configures
// (agents/planner/llm/default.yaml). The behavioral shipped-profile run gates on
// this model being served by Ollama.
const plannerModel = "qwen3.6:35b-mlx"

const plannerFirstResponseFixture = `title: Align planner prompts
summary: Make every planner prompt request the parser-owned plan schema.
files:
  - path: applications/catalog/agents/planner/builtin.yaml
    action: modify
    note: Declare the canonical output document.
requirements:
  - id: R1
    text: Request one top-level implementation-plan mapping.
design_decisions:
  - id: D1
    text: Keep response-shape policy in profile-owned prompts.
acceptance_criteria:
  - id: AC1
    text: The first model response parses without retry.
`

// machineTransition is one transition row of a shipped machine.yaml, enough to
// assert the wiring the conformance tests care about.
type machineTransition struct {
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action"`
	Label  string `yaml:"label"`
}

type plannerToolDeclaration struct {
	Name        string         `yaml:"name"`
	Type        string         `yaml:"type"`
	Init        string         `yaml:"init"`
	StdinSource string         `yaml:"stdin_source"`
	Config      map[string]any `yaml:"config"`
	Output      struct {
		Mode string `yaml:"mode"`
	} `yaml:"output"`
}

// TestPlannerShippedProfileWiring asserts, model-free and ungated, that the
// wrapper an operator ships — agents/planner/profile.yaml with its machine.yaml
// — wires the requirement-graph boundary: the profile references its own
// machine and tools, the machine seeds from Idle with load_graph, and the loaded
// graph hands off to extraction. This is the load + machine-wiring proof for the
// shipped wrapper; unlike the behavioral run below it needs no model, so it runs
// in the fast default and holds even where Ollama is absent.
//
// Traces srd004-planner AC1 (load_graph as the pipeline's graph-boundary action).
func TestPlannerShippedProfileWiring(t *testing.T) {
	var profile struct {
		Machine string   `yaml:"machine"`
		Tools   []string `yaml:"tools"`
	}
	unmarshalShipped(t, filepath.Join("agents", "planner", "profile.yaml"), &profile)

	if profile.Machine != "machine.yaml" {
		t.Errorf("shipped planner profile machine = %q, want machine.yaml", profile.Machine)
	}
	if !contains(profile.Tools, "tools.yaml") {
		t.Errorf("shipped planner profile tools = %v, want to include tools.yaml", profile.Tools)
	}

	var machine struct {
		InitialState string              `yaml:"initial_state"`
		Transitions  []machineTransition `yaml:"transitions"`
	}
	unmarshalShipped(t, filepath.Join("agents", "planner", "machine.yaml"), &machine)

	if machine.InitialState != "Idle" {
		t.Errorf("shipped planner machine initial_state = %q, want Idle", machine.InitialState)
	}
	// The graph-loading boundary: Idle seeds load_graph, and the loaded graph
	// hands off to task extraction.
	requireTransition(t, machine.Transitions, "Idle", "Seed", "Loading", "load_graph")
	requireTransition(t, machine.Transitions, "Loading", "GraphLoaded", "Extracting", "extract_task")
}

// TestPlannerRetryPolicyWiring proves GH-885's policy is authored in both
// shipped machines: command state owns the counter, value_predicate owns the
// limit comparison, and focused graph words own completion and querying.
func TestPlannerRetryPolicyWiring(t *testing.T) {
	var selected struct {
		Tools []string `yaml:"tools"`
	}
	unmarshalShipped(t, filepath.Join("agents", "planner", "tools.yaml"), &selected)
	for _, tool := range []string{
		"reset_retry_count", "increment_retry", "check_retry_limit",
		"mark_task_done", "mark_task_failed", "remaining_work",
	} {
		if !contains(selected.Tools, tool) {
			t.Errorf("planner tools omit %q", tool)
		}
	}

	var declarations struct {
		Tools []plannerToolDeclaration `yaml:"tools"`
	}
	unmarshalShipped(t, filepath.Join("agents", "planner", "builtin.yaml"), &declarations)
	byName := make(map[string]plannerToolDeclaration)
	for _, tool := range declarations.Tools {
		byName[tool.Name] = tool
	}
	increment, ok := byName["increment_retry"]
	if !ok || increment.StdinSource != "$from(retry_count).output" || increment.Output.Mode != "structured" {
		t.Fatalf("increment_retry does not consume and republish labelled retry_count: %#v", byName["increment_retry"])
	}
	predicate, ok := byName["check_retry_limit"]
	if !ok || predicate.Init != "value_predicate" ||
		fmt.Sprint(predicate.Config["op"]) != "lt" ||
		fmt.Sprint(predicate.Config["right"]) != "2" ||
		fmt.Sprint(predicate.Config["satisfied"]) != "RetryAvailable" ||
		fmt.Sprint(predicate.Config["unsatisfied"]) != "RetriesExhausted" {
		t.Fatalf("check_retry_limit does not declare first-retry/exhaustion policy: %#v", byName["check_retry_limit"])
	}

	for _, machinePath := range []string{"machine.yaml", "machine-passthrough.yaml"} {
		t.Run(machinePath, func(t *testing.T) {
			var machine struct {
				Transitions []machineTransition `yaml:"transitions"`
			}
			unmarshalShipped(t, filepath.Join("agents", "planner", machinePath), &machine)

			if machinePath == "machine.yaml" {
				requireTransition(t, machine.Transitions, "Extracting", "TaskExtracted", "MarkingPlanning", "mark_nodes_planning")
				requireLabeledTransition(t, machine.Transitions, "MarkingPlanning", "NodesMarkedPlanning", "InitializingFailureContext", "reset_failure_context", "failure_context")
				requireLabeledTransition(t, machine.Transitions, "InitializingFailureContext", "FailureContextInitialized", "InitializingRetry", "reset_retry_count", "retry_count")
			} else {
				requireTransition(t, machine.Transitions, "Loading", "GraphLoaded", "SelectingReady", "select_all_ready")
				requireTransition(t, machine.Transitions, "SelectingReady", "ReadySelected", "SeedingPassThroughPlan", "seed_passthrough_plan")
				requireTransition(t, machine.Transitions, "SeedingPassThroughPlan", "PassThroughPlanSeeded", "MarkingPlanning", "mark_nodes_planning")
				requireLabeledTransition(t, machine.Transitions, "MarkingPlanning", "NodesMarkedPlanning", "InitializingRetry", "reset_retry_count", "retry_count")
			}
			if machinePath == "machine.yaml" {
				requireLabeledTransition(t, machine.Transitions, "Testing", "ToolFailed", "CapturingFailure", "capture_planner_failure", "failure_context")
				requireLabeledTransition(t, machine.Transitions, "CapturingFailure", "FailureCaptured", "IncrementingRetry", "increment_retry", "retry_count")
			} else {
				requireLabeledTransition(t, machine.Transitions, "Testing", "ToolFailed", "IncrementingRetry", "increment_retry", "retry_count")
			}
			requireTransition(t, machine.Transitions, "IncrementingRetry", "ToolDone", "CheckingRetryLimit", "check_retry_limit")
			requireTransition(t, machine.Transitions, "CheckingRetryLimit", "RetriesExhausted", "MarkingTaskFailed", "mark_task_failed")
			requireTransition(t, machine.Transitions, "MarkingTaskFailed", "TaskFailed", "Exhausted", "remaining_work")
			requireTransition(t, machine.Transitions, "Testing", "ToolDone", "MarkingTaskDone", "mark_task_done")
			requireTransition(t, machine.Transitions, "MarkingTaskDone", "TaskCompleted", "QueryingRemainingWork", "remaining_work")
			requireTransition(t, machine.Transitions, "Exhausted", "Blocked", "Stalled", "")
			requireTransition(t, machine.Transitions, "QueryingRemainingWork", "AllDone", "Completed", "")
			requireTransition(t, machine.Transitions, "QueryingRemainingWork", "Blocked", "Stalled", "")
			if machinePath == "machine.yaml" {
				requireTransition(t, machine.Transitions, "CheckingRetryLimit", "RetryAvailable", "Resetting", "reset_history")
				requireTransition(t, machine.Transitions, "QueryingRemainingWork", "WorkRemaining", "Extracting", "extract_task")
			} else {
				requireTransition(t, machine.Transitions, "CheckingRetryLimit", "RetryAvailable", "InvokingExecutor", "invoke_executor")
				requireTransition(t, machine.Transitions, "QueryingRemainingWork", "WorkRemaining", "Completed", "")
			}
		})
	}
}

// TestPlannerCanonicalPlanFirstResponse serves one deterministic response with
// the complete profile-owned ImplementationPlan schema. The shipped load,
// extraction, prompt composition, model boundary, and parse_plan word remain in
// control; only the post-parse transition is bounded to a successful terminal.
func TestPlannerCanonicalPlanFirstResponse(t *testing.T) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(plannerFirstResponseFixture), &document); err != nil {
		t.Fatalf("unmarshal planner response fixture: %v", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("planner response root kind = %v, want exactly one mapping", document.Content)
	}
	root := document.Content[0]
	fields := make(map[string]bool)
	for index := 0; index < len(root.Content); index += 2 {
		fields[root.Content[index].Value] = true
	}
	wantFields := []string{
		"title", "summary", "files", "requirements",
		"design_decisions", "acceptance_criteria",
	}
	if len(fields) != len(wantFields) {
		t.Fatalf("planner response fields = %v, want exactly %v", fields, wantFields)
	}
	for _, field := range wantFields {
		if !fields[field] {
			t.Errorf("planner response omits canonical field %q", field)
		}
	}

	var chatCalls atomic.Int32
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.6:35b-mlx"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/chat":
			chatCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]string{
					"role":    "assistant",
					"content": plannerFirstResponseFixture,
				},
				"eval_count":        24,
				"prompt_eval_count": 48,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	profile := CopyShippedProfile(t, filepath.Join("agents", "planner", "profile.yaml"), map[string]string{
		"http://localhost:11434": ollama.URL,
		`- state: PlanParsing
  signal: PlanReady
  next: IssueFormatting
  action: format_issue
  label: issue_input`: `- state: PlanParsing
  signal: PlanReady
  next: Completed`,
	})
	coreRoot := RequireCoreRoot(t)
	workspace := filepath.Join(coreRoot, "pkg", "spec", "testdata", "valid")
	result := Run(t, RunConfig{Profile: profile, Directory: workspace})

	result.RequireExit(t, 0)
	result.RequireNoErrorSpans(t)
	result.RequireTerminalState(t, "Completed")
	result.RequireToolSpans(t,
		"load_graph", "extract_task", "compose_planner_prompt", "parse_plan",
	)
	if got := len(result.Spans.Named("chat " + plannerModel)); got == 0 {
		t.Fatalf("missing invoke_llm chat span; span names: %v", result.Spans.Names())
	}
	if got := chatCalls.Load(); got != 1 {
		t.Fatalf("model response count = %d, want exactly one first response", got)
	}
	if got := len(result.Spans.Named("execute_tool parse_plan")); got != 1 {
		t.Fatalf("parse_plan span count = %d, want exactly one", got)
	}
	if got := len(result.Spans.Named("execute_tool report_parse_error")); got != 0 {
		t.Fatalf("report_parse_error ran after canonical first response: %d spans", got)
	}
	if _, _, ok := result.Spans.FindEvent("pipeline.plan_parsed"); !ok {
		t.Fatalf("no pipeline.plan_parsed event; span names: %v\noutput:\n%s", result.Spans.Names(), result.Output)
	}
}

// TestPlannerInvalidThenCanonicalPlan proves the shipped planner's declared
// response contract corrects one invalid response in the parse_plan domain.
// The second model turn must receive the exact YAML mapping feedback and the
// canonical response must reach PlanReady without another correction.
func TestPlannerInvalidThenCanonicalPlan(t *testing.T) {
	const wantFeedback = "Your previous response was invalid. parse plan: missing required field: requirements (list is empty)\n\n" +
		"Please respond with exactly one top-level YAML mapping and no other document content. " +
		"The mapping must contain exactly these six keys: title, summary, files, requirements, " +
		"design_decisions, and acceptance_criteria. Do not return a root sequence/list, multiple " +
		"plans, a wrapper/envelope key, Markdown fences, prose, or any keys outside this mapping."

	var chatCalls atomic.Int32
	var correction atomic.Value
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.6:35b-mlx"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/chat":
			var request struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode planner chat request: %v", err)
			}
			call := chatCalls.Add(1)
			content := "title: Missing required lists\n"
			if call == 2 {
				for index := len(request.Messages) - 1; index >= 0; index-- {
					if request.Messages[index].Role == "user" {
						correction.Store(request.Messages[index].Content)
						break
					}
				}
				content = plannerFirstResponseFixture
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message":    map[string]string{"role": "assistant", "content": content},
				"eval_count": 24, "prompt_eval_count": 48,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()

	profile := CopyShippedProfile(t, filepath.Join("agents", "planner", "profile.yaml"), map[string]string{
		"http://localhost:11434": ollama.URL,
		`- state: PlanParsing
  signal: PlanReady
  next: IssueFormatting
  action: format_issue
  label: issue_input`: `- state: PlanParsing
  signal: PlanReady
  next: Completed`,
	})
	coreRoot := RequireCoreRoot(t)
	workspace := filepath.Join(coreRoot, "pkg", "spec", "testdata", "valid")
	result := Run(t, RunConfig{Profile: profile, Directory: workspace})

	result.RequireExit(t, 0)
	result.RequireNoErrorSpans(t)
	result.RequireTerminalState(t, "Completed")
	if got := chatCalls.Load(); got != 2 {
		t.Fatalf("model response count = %d, want invalid response plus one correction", got)
	}
	if got := correction.Load(); got != wantFeedback {
		t.Fatalf("planner correction feedback:\n%v\nwant:\n%s", got, wantFeedback)
	}
	if got := len(result.Spans.Named("execute_tool parse_plan")); got != 2 {
		t.Fatalf("parse_plan span count = %d, want invalid plus canonical parse", got)
	}
	if got := len(result.Spans.Named("execute_tool report_parse_error")); got != 1 {
		t.Fatalf("report_parse_error span count = %d, want exactly one correction", got)
	}
	if _, _, ok := result.Spans.FindEvent("pipeline.plan_parsed"); !ok {
		t.Fatalf("canonical correction did not reach PlanReady; output:\n%s", result.Output)
	}
}

// TestPlannerConformance runs the shipped planner profile against agent-core's
// valid spec fixture and asserts the requirement-graph boundary from the trace:
// load_graph reads the corpus and builds the requirement graph into pipeline
// state (the pipeline.graph_loaded event), the boundary that the #211 nil-graph
// gap was about. This runs the wrapper an operator ships — not a synthesized,
// bounded machine — so it exercises the real planner tool declarations.
//
// It is Ollama-gated: the conformance harness probes the profile's effective
// provider URL and required model before launching the shipped planner. The
// full pipeline tail beyond the graph boundary
// (project_planner_context -> compose_planner_prompt -> invoke_llm -> parse_plan -> profile-selected tracker
// sentence -> write -> self_invoke via an executor child -> vet/build/test) needs a
// tracker project, a child agent, and the Go toolchain, which the conformance harness
// deliberately does not provide; the shipped planner is therefore behaviorally
// exercised to its requirement-graph boundary here and no further, so no clean
// terminal is asserted. The remaining boundary wiring is proven ungated by
// TestPlannerShippedProfileWiring.
//
// Traces srd004-planner: AC1 (load_graph as the graph-boundary action) and AC2
// (the requirement graph is built into pipeline state).
func TestPlannerConformance(t *testing.T) {
	liveTimeout := RequireLiveModel(t, ollamaURLFromEnvironment(), plannerModel)
	coreRoot := RequireCoreRoot(t)

	corpus := filepath.Join(coreRoot, "pkg", "spec", "testdata", "valid")

	result := Run(t, RunConfig{
		Profile:   filepath.Join("agents", "planner", "profile.yaml"),
		Directory: corpus,
		Timeout:   liveTimeout,
	})

	// srd004 AC1: the shipped wrapper runs under a single root and selects
	// load_graph as its first, graph-boundary action.
	result.RootRequired(t)
	result.RequireToolSpans(t, "load_graph", "extract_task")

	// srd004 AC2: load_graph seeded the requirement graph into pipeline state.
	if _, _, ok := result.Spans.FindEvent("pipeline.graph_loaded"); !ok {
		t.Fatalf("no pipeline.graph_loaded event; span names: %v\noutput:\n%s", result.Spans.Names(), result.Output)
	}
}

// TestPlannerShippedProfileTerminalExecution runs the shipped pass-through
// planner variant through its real write and self_invoke commands. A controlled
// child executable isolates the process boundary while the shipped profile,
// declarations, graph loader, extractor, validators, and transition table remain
// in control of command selection and terminal state mapping.
func TestPlannerShippedProfileTerminalExecution(t *testing.T) {
	coreRoot := RequireCoreRoot(t)
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.6:35b-mlx"}]}`))
	}))
	defer ollama.Close()

	profile := CopyShippedProfile(t, filepath.Join("agents", "planner", "profile.yaml"), map[string]string{
		"machine: machine.yaml":  "machine: machine-passthrough.yaml",
		"http://localhost:11434": ollama.URL,
	})
	workspace := t.TempDir()
	requireCopyFS(t, workspace, filepath.Join(coreRoot, "pkg", "spec", "testdata", "valid"))
	writeEphemeral(t, workspace, "go.mod", "module plannerproof\n\ngo 1.26\n")
	writeEphemeral(t, workspace, "plannerproof_test.go", "package plannerproof\n\nimport \"testing\"\n\nfunc TestProof(t *testing.T) {}\n")

	childArgs := filepath.Join(t.TempDir(), "child-args.txt")
	child := filepath.Join(t.TempDir(), "agent")
	writeEphemeral(t, filepath.Dir(child), filepath.Base(child), fmt.Sprintf(
		"#!/bin/sh\nset -eu\nprintf '%%s\\n' \"$*\" > %q\n", childArgs,
	))
	if err := os.Chmod(child, 0o755); err != nil {
		t.Fatalf("chmod controlled child: %v", err)
	}

	result := Run(t, RunConfig{
		Profile: profile, Directory: workspace,
		Args: []string{"--child-agent-binary", child},
	})

	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireToolSpans(t, "load_graph", "select_all_ready", "seed_passthrough_plan", "mark_nodes_planning", "reset_retry_count", "mark_nodes_executing", "format_task_file", "write", "invoke_executor", "vet", "build", "test", "mark_task_done", "remaining_work")
	result.RequireTerminalState(t, "Completed")
	args := readFile(t, childArgs)
	if !strings.Contains(args, "--profile agents/executor/profile.yaml") {
		t.Fatalf("self_invoke child args do not select shipped executor profile:\n%s", args)
	}
}

// TestPlannerShippedProfileRetryExhaustion executes the pass-through profile
// with a permanently failing validator. The second execute proves the first
// failure took RetryAvailable; the Stalled terminal after exactly two
// increments proves the declared limit took RetriesExhausted.
func TestPlannerShippedProfileRetryExhaustion(t *testing.T) {
	coreRoot := RequireCoreRoot(t)
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.6:35b-mlx"}]}`))
	}))
	defer ollama.Close()

	profile := CopyShippedProfile(t, filepath.Join("agents", "planner", "profile.yaml"), map[string]string{
		"machine: machine.yaml":  "machine: machine-passthrough.yaml",
		"http://localhost:11434": ollama.URL,
	})
	workspace := t.TempDir()
	requireCopyFS(t, workspace, filepath.Join(coreRoot, "pkg", "spec", "testdata", "valid"))
	writeEphemeral(t, workspace, "go.mod", "module plannerretry\n\ngo 1.26\n")
	writeEphemeral(t, workspace, "plannerretry_test.go", "package plannerretry\n\nimport \"testing\"\n\nfunc TestFailure(t *testing.T) { t.Fatal(\"retry proof\") }\n")

	child := filepath.Join(t.TempDir(), "agent")
	writeEphemeral(t, filepath.Dir(child), filepath.Base(child), "#!/bin/sh\nset -eu\n")
	if err := os.Chmod(child, 0o755); err != nil {
		t.Fatalf("chmod controlled child: %v", err)
	}

	result := Run(t, RunConfig{
		Profile: profile, Directory: workspace,
		Args: []string{"--child-agent-binary", child},
	})

	result.RequireExit(t, 2)
	result.RequireTerminalState(t, "Stalled")
	result.RequireToolSpans(t, "increment_retry", "check_retry_limit", "remaining_work")
	result.RequireToolSpans(t, "mark_task_failed")
	if got := len(result.Spans.Named("execute_tool invoke_executor")); got != 2 {
		t.Fatalf("invoke_executor span count = %d, want 2 (initial attempt plus first retry)", got)
	}
	if got := len(result.Spans.Named("execute_tool increment_retry")); got != 2 {
		t.Fatalf("increment_retry span count = %d, want 2", got)
	}
	signals := toolSignalCounts(result, "check_retry_limit")
	if signals["RetryAvailable"] != 1 || signals["RetriesExhausted"] != 1 {
		t.Fatalf("check_retry_limit signals = %v, want one RetryAvailable and one RetriesExhausted", signals)
	}
	if got := len(result.Spans.Named("execute_tool mark_task_done")); got != 0 {
		t.Fatalf("mark_task_done ran on exhausted validation: %d spans", got)
	}
}

func toolSignalCounts(result RunResult, tool string) map[string]int {
	counts := make(map[string]int)
	for _, span := range result.Spans.Named("execute_tool " + tool) {
		for _, attr := range span.Attributes {
			if attr.Key != "command.signal" {
				continue
			}
			if signal, ok := attr.Value.Value.(string); ok {
				counts[signal]++
			}
		}
	}
	return counts
}

func requireCopyFS(t *testing.T, destination, source string) {
	t.Helper()
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatalf("copy planner proof corpus: %v", err)
	}
}

// unmarshalShipped reads a shipped YAML file (path relative to the agent-profiles
// root) and unmarshals it into out.
func unmarshalShipped(t *testing.T, rel string, out any) {
	t.Helper()
	data, err := os.ReadFile(ProfilePath(rel))
	if err != nil {
		t.Fatalf("read shipped %s: %v", rel, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal shipped %s: %v", rel, err)
	}
}

// requireTransition fails unless transitions contains an entry matching the
// given state, signal, next state, and action.
func requireTransition(t *testing.T, transitions []machineTransition, state, signal, next, action string) {
	t.Helper()
	for _, tr := range transitions {
		if tr.State == state && tr.Signal == signal {
			if tr.Next != next {
				t.Errorf("transition %s/%s next = %q, want %q", state, signal, tr.Next, next)
			}
			if tr.Action != action {
				t.Errorf("transition %s/%s action = %q, want %q", state, signal, tr.Action, action)
			}
			return
		}
	}
	t.Errorf("no transition for state %q signal %q found", state, signal)
}

func requireLabeledTransition(t *testing.T, transitions []machineTransition, state, signal, next, action, label string) {
	t.Helper()
	for _, tr := range transitions {
		if tr.State == state && tr.Signal == signal {
			requireTransition(t, transitions, state, signal, next, action)
			if tr.Label != label {
				t.Errorf("transition %s/%s label = %q, want %q", state, signal, tr.Label, label)
			}
			return
		}
	}
	t.Errorf("no transition for state %q signal %q found", state, signal)
}

// contains reports whether s is in list.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
