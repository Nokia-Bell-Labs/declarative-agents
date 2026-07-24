// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// These cover the creator's corpus-ingest child run (srd005 R3.1, GH-762): the
// creator runs the agent binary against the corpus-ingest profile (agent-core
// srd021), then reads the collection back to report what the run wrote. Before
// this, every operation -- ingest included -- drove apply_instance, so an ingest
// was a values apply wearing a label and no corpus-ingest instance was ever
// created (GH-755 established that).

func creatorIngestEndpoint(t *testing.T) rolloutEndpoint {
	t.Helper()
	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)
	endpoint, ok := rest.Rest.Servers["creator_instance"].Endpoints["ingest"]
	if !ok {
		t.Fatal("creator declares no ingest endpoint; the coordinator has nothing to delegate a corpus-ingest run to")
	}
	return endpoint
}

// TestCreatorIngestIsItsOwnEndpoint proves the ingest does not ride on the
// instance endpoint. initial_signal is per-endpoint, so a machine cannot branch
// on a body field: an ingest posted at /instance could only ever take the
// values-apply leg, which is exactly how it broke.
func TestCreatorIngestIsItsOwnEndpoint(t *testing.T) {
	ingest := creatorIngestEndpoint(t)
	if ingest.Method != "POST" {
		t.Errorf("ingest method = %q, want POST", ingest.Method)
	}
	if ingest.MachineRequest.InitialSignal != "SeedIngest" {
		t.Errorf("ingest initial_signal = %q, want SeedIngest", ingest.MachineRequest.InitialSignal)
	}
	if strings.HasPrefix(ingest.Path, "/provisioning") {
		t.Errorf("ingest path %q is on the browser prefix; the creator is coordinator-facing only (srd005 R5.4)", ingest.Path)
	}

	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)
	instance := rest.Rest.Servers["creator_instance"].Endpoints["instance"]
	if ingest.MachineRequest.Machine != instance.MachineRequest.Machine {
		t.Errorf("ingest uses machine %q and instance uses %q; both legs live in the one request machine",
			ingest.MachineRequest.Machine, instance.MachineRequest.Machine)
	}
}

// TestCreatorIngestLegMeasuresChildDelta proves the leg brackets the child with
// count reads. The child's exit code alone does not report the outcome:
// corpus-ingest reaches its own Succeeded terminal on CountReady whatever the
// count, and an accumulated collection total does not identify this run's work.
func TestCreatorIngestLegMeasuresChildDelta(t *testing.T) {
	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-machine.yaml"), &machine)

	want := []struct{ state, signal, action, next string }{
		{"AwaitingRequest", "SeedIngest", "resolve_ingest_collection", "ResolvingIngested"},
		{"ResolvingIngested", "CollectionResolved", "count_before_ingest", "CountingBeforeIngest"},
		{"CountingBeforeIngest", "DocumentsCounted", "run_corpus_ingest", "Ingesting"},
		{"Ingesting", "ToolDone", "count_after_ingest", "CountingAfterIngest"},
		{"CountingAfterIngest", "DocumentsCounted", "ingest_wrote_documents", "JudgingIngest"},
	}
	for _, w := range want {
		var found bool
		for _, tr := range machine.Transitions {
			if tr.State != w.state || tr.Signal != w.signal {
				continue
			}
			found = true
			if tr.Action != w.action {
				t.Errorf("%s + %s action = %q, want %q", w.state, w.signal, tr.Action, w.action)
			}
			if tr.Next != w.next {
				t.Errorf("%s + %s next = %q, want %q", w.state, w.signal, tr.Next, w.next)
			}
		}
		if !found {
			t.Errorf("no %s + %s transition; the ingest leg is incomplete", w.state, w.signal)
		}
	}

	// A failed child and an unreadable collection both have to land somewhere;
	// an unmapped terminal answers response_missing at runtime.
	for _, state := range []string{"ResolvingIngested", "CountingBeforeIngest", "Ingesting", "CountingAfterIngest"} {
		var reaches bool
		for _, tr := range machine.Transitions {
			if tr.State == state && tr.Next == "IngestFailed" {
				reaches = true
			}
		}
		if !reaches {
			t.Errorf("%s has no path to IngestFailed; a failure there would fall through", state)
		}
	}
}

// TestCreatorIngestMapsItsTerminals proves every terminal the leg can reach maps
// to a response. An unmapped terminal answers response_missing at runtime.
func TestCreatorIngestMapsItsTerminals(t *testing.T) {
	states := creatorIngestEndpoint(t).MachineRequest.Response.TerminalStates
	if len(states) == 0 {
		t.Fatal("ingest maps no terminal states")
	}

	ingested, ok := states["Ingested"]
	if !ok {
		t.Fatal("ingest does not map Ingested")
	}
	if ingested.Status != 200 {
		t.Errorf("Ingested status = %d, want 200", ingested.Status)
	}
	// ingest_wrote_documents is the terminal word. Its raw left operand is the
	// post-run count, so the response must select that field rather than losing
	// the judged value at the terminal boundary (agent-core srd041 R4.4).
	if got := ingested.Body["count"]; got != "$.left" {
		t.Errorf("Ingested count = %q, want terminal predicate left operand $.left", got)
	}

	failed, ok := states["IngestFailed"]
	if !ok {
		t.Fatal("ingest does not map IngestFailed; a failed child would fall through")
	}
	if failed.Status < 400 {
		t.Errorf("IngestFailed status = %d, which the coordinator reads as success", failed.Status)
	}

	// Every terminal the machine declares must be mapped, and nothing else.
	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-machine.yaml"), &machine)
	ingestTerminals := map[string]bool{"Ingested": true, "IngestRejected": true, "IngestFailed": true}
	for state := range states {
		if !ingestTerminals[state] {
			t.Errorf("ingest maps unexpected terminal %q", state)
		}
		if !containsString(machine.TerminalStates, state) {
			t.Errorf("ingest maps %q, which the machine does not declare terminal", state)
		}
	}
	for want := range ingestTerminals {
		if _, ok := states[want]; !ok {
			t.Errorf("ingest does not map terminal %q", want)
		}
	}
}

// TestCorpusIngestChildRunFollowsSrd021 proves the child-run word is the
// agent-as-child-CLI shape (agent-core srd021 R1): the agent binary, a profile
// the child owns, and the requested directory as its workspace. launch_eval and
// run_agent are not reusable -- both are bench-harness words needing a suite path
// or a point workspace.
func TestCorpusIngestChildRunFollowsSrd021(t *testing.T) {
	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-declarations.yaml"), &decls)
	var word execDeclaration
	for _, tool := range decls.Tools {
		if tool.Name == "run_corpus_ingest" {
			word = tool
		}
	}
	if word.Name == "" {
		t.Fatal("the creator declares no run_corpus_ingest word")
	}

	if word.Binary != "agent" {
		t.Errorf("binary = %q, want agent (srd021 R1.1)", word.Binary)
	}
	joined := strings.Join(word.Args, " ")
	for _, want := range []string{"--profile", "corpus-ingest", "--core-root"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %v omit %q", word.Args, want)
		}
	}
	// The child profile owns every program-shaping path (srd021 R2.2), so the
	// parent must not pass a machine or tool selection of its own.
	for _, forbidden := range []string{"--machine", "--tools"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("args %v pass %q; the child profile owns program shaping (srd021 R1.3, R2.2)", word.Args, forbidden)
		}
	}
	for _, signal := range []string{"ToolDone", "ToolFailed"} {
		if !containsString(word.Emits, signal) {
			t.Errorf("emits %v missing %s", word.Emits, signal)
		}
	}
}

// TestCreatorIngestJudgesTheCountBeforeReporting proves the shortfall decision is
// a declared word in the machine rather than a check inside the child or a Go
// branch (GH-763). corpus-ingest reaches its own Succeeded terminal whatever the
// count, so without this leg a run that wrote nothing reported success.
func TestCreatorIngestJudgesTheCountBeforeReporting(t *testing.T) {
	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-machine.yaml"), &machine)

	want := []struct{ state, signal, action, next string }{
		{"CountingAfterIngest", "DocumentsCounted", "ingest_wrote_documents", "JudgingIngest"},
		{"JudgingIngest", "DocumentsWritten", "", "Ingested"},
		{"JudgingIngest", "NoDocumentsWritten", "", "IngestRejected"},
		// A count that will not resolve is a fault, not an empty corpus. Routing
		// it to IngestRejected would tell the coordinator a directory held no
		// documents when nobody looked (agent-core srd041 R4.2).
		{"JudgingIngest", "CommandError", "", "IngestFailed"},
	}
	for _, w := range want {
		var found bool
		for _, tr := range machine.Transitions {
			if tr.State != w.state || tr.Signal != w.signal {
				continue
			}
			found = true
			if tr.Action != w.action {
				t.Errorf("%s + %s action = %q, want %q", w.state, w.signal, tr.Action, w.action)
			}
			if tr.Next != w.next {
				t.Errorf("%s + %s next = %q, want %q", w.state, w.signal, tr.Next, w.next)
			}
		}
		if !found {
			t.Errorf("no %s + %s transition; the shortfall leg is incomplete", w.state, w.signal)
		}
	}
}

// TestCreatorIngestPopulatedCollectionUsesRunDelta covers the failure mode from
// GH-772. A post-run total above zero is insufficient when the collection was
// already populated; success requires the post-run count to exceed the pre-run
// count preserved in command state.
func TestCreatorIngestPopulatedCollectionUsesRunDelta(t *testing.T) {
	var decls struct {
		Tools []struct {
			Name   string `yaml:"name"`
			Config struct {
				RestRef   string `yaml:"rest_ref"`
				Operation string `yaml:"operation"`
				Left      string `yaml:"left"`
				Op        string `yaml:"op"`
				Right     string `yaml:"right"`
			} `yaml:"config"`
		} `yaml:"tools"`
	}
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-declarations.yaml"), &decls)

	tools := make(map[string]struct {
		RestRef   string
		Operation string
		Left      string
		Op        string
		Right     string
	})
	for _, tool := range decls.Tools {
		tools[tool.Name] = struct {
			RestRef   string
			Operation string
			Left      string
			Op        string
			Right     string
		}{
			RestRef: tool.Config.RestRef, Operation: tool.Config.Operation,
			Left: tool.Config.Left, Op: tool.Config.Op, Right: tool.Config.Right,
		}
	}

	for _, name := range []string{"count_before_ingest", "count_after_ingest"} {
		count, ok := tools[name]
		if !ok {
			t.Fatalf("creator declares no %s word", name)
		}
		if count.RestRef != "chroma" || count.Operation != "count_ingested_documents" {
			t.Errorf("%s invokes %s.%s, want chroma.count_ingested_documents", name, count.RestRef, count.Operation)
		}
	}

	predicate, ok := tools["ingest_wrote_documents"]
	if !ok {
		t.Fatal("creator declares no ingest_wrote_documents predicate")
	}
	if predicate.Left != "$.mapped.count" || predicate.Op != "gt" ||
		predicate.Right != "$from(count_before_ingest).mapped.count" {
		t.Errorf("ingest predicate = %q %s %q, want post-run count gt $from(count_before_ingest).mapped.count",
			predicate.Left, predicate.Op, predicate.Right)
	}

	var rest struct {
		Rest struct {
			Clients map[string]struct {
				Operations map[string]struct {
					Params struct {
						BodySource   string            `yaml:"body_source"`
						InputMapping map[string]string `yaml:"input_mapping"`
					} `yaml:"params"`
				} `yaml:"operations"`
			} `yaml:"clients"`
		} `yaml:"rest"`
	}
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)
	params := rest.Rest.Clients["chroma"].Operations["count_ingested_documents"].Params
	if params.BodySource != "command_state" {
		t.Errorf("count body_source = %q, want command_state", params.BodySource)
	}
	if got := params.InputMapping["collection"]; got != "$from(resolve_ingest_collection).mapped.collection_id" {
		t.Errorf("count collection selector = %q, want resolved collection from command state", got)
	}
}

// TestCreatorIngestShortfallIsAClientError proves a run that wrote nothing is a
// 422 and a broken run a 500, which is the distinction the coordinator's declared
// failure mapping needs.
func TestCreatorIngestShortfallIsAClientError(t *testing.T) {
	states := creatorIngestEndpoint(t).MachineRequest.Response.TerminalStates

	rejected, ok := states["IngestRejected"]
	if !ok {
		t.Fatal("ingest does not map IngestRejected; the coordinator's Rejected leg stays unreachable")
	}
	if rejected.Status != 422 {
		t.Errorf("IngestRejected status = %d, want 422; a shortfall is the caller's source falling short, not our failure", rejected.Status)
	}
	if failed := states["IngestFailed"]; failed.Status != 500 {
		t.Errorf("IngestFailed status = %d, want 500", failed.Status)
	}
}

// TestCoordinatorIngestReachesEndpointAndMapsCount proves the delegation targets
// the endpoint that runs a child and carries its judged post-run count forward.
// It posted at /api/v1/instance until GH-763, where every operation took the
// values-apply leg whatever it named, so this hop ran no ingest at all.
func TestCoordinatorIngestReachesEndpointAndMapsCount(t *testing.T) {
	op := clientOperationNamed(t, "coordinator", "creator", "creator_ingest")
	if op.Path != "/api/v1/ingest" {
		t.Errorf("creator_ingest path = %q, want /api/v1/ingest", op.Path)
	}
	if op.Success.Signal != "IngestReported" {
		t.Errorf("success signal = %q, want IngestReported", op.Success.Signal)
	}
	if got := op.Response.Output["count"]; got != "$.count" {
		t.Errorf("creator_ingest count mapping = %q, want $.count", got)
	}
	// The 422 must map to Rejected, or a shortfall would surface as an
	// unmapped status and collapse into CommandError.
	var rejects bool
	var rest struct {
		Rest struct {
			Clients map[string]struct {
				Operations map[string]struct {
					Failures []struct {
						Status []int  `yaml:"status"`
						Signal string `yaml:"signal"`
					} `yaml:"failures"`
				} `yaml:"operations"`
			} `yaml:"clients"`
		} `yaml:"rest"`
	}
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "rest.yaml"), &rest)
	for _, f := range rest.Rest.Clients["creator"].Operations["creator_ingest"].Failures {
		for _, s := range f.Status {
			if s == 422 && f.Signal == "Rejected" {
				rejects = true
			}
		}
	}
	if !rejects {
		t.Error("creator_ingest does not map 422 to Rejected; a shortfall would arrive as an unmapped status")
	}
}
