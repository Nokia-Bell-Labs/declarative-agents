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

// TestCreatorIngestLegRunsAChildThenCounts proves the leg does both halves. The
// child's exit code alone does not report the outcome: corpus-ingest reaches its
// own Succeeded terminal on CountReady whatever the count, so a run that wrote
// nothing still exits zero. Only the collection read says what landed.
func TestCreatorIngestLegRunsAChildThenCounts(t *testing.T) {
	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-machine.yaml"), &machine)

	want := []struct{ state, signal, action, next string }{
		{"AwaitingRequest", "SeedIngest", "run_corpus_ingest", "Ingesting"},
		{"Ingesting", "ToolDone", "resolve_ingest_collection", "ResolvingIngested"},
		{"ResolvingIngested", "CollectionResolved", "count_ingested_documents", "CountingIngested"},
		{"CountingIngested", "DocumentsCounted", "", "Ingested"},
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
	for _, state := range []string{"Ingesting", "ResolvingIngested", "CountingIngested"} {
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

// TestCreatorIngestMapsItsTerminals proves the two outcomes map to distinct
// responses, and that no 422 is claimed. Rejecting a shortfall is a comparison
// against zero, and no declarative primitive expresses one -- response schema
// validation supports only required and type, and no builtin emits a signal from
// a value test -- so the coordinator's Rejected leg stays honestly unreachable
// rather than being faked here.
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
	// $.mapped.count, not $.count: a REST word's output arrives nested under
	// mapped, and the un-prefixed selector silently served count: null.
	if got := ingested.Body["count"]; got != "$.mapped.count" {
		t.Errorf("Ingested body count = %q, want $.mapped.count; the un-prefixed selector resolves to null", got)
	}

	failed, ok := states["IngestFailed"]
	if !ok {
		t.Fatal("ingest does not map IngestFailed; a failed child would fall through")
	}
	if failed.Status < 400 {
		t.Errorf("IngestFailed status = %d, which the coordinator reads as success", failed.Status)
	}

	for state := range states {
		if state == "Ingested" || state == "IngestFailed" {
			continue
		}
		t.Errorf("ingest maps unexpected terminal %q", state)
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
