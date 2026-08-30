// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpusIngestCollectionVar is the environment variable the child reads for the
// collection it writes into. The creator overlays it per run, so the name is the
// seam between the two: a rename on one side alone sends the child somewhere the
// creator does not count.
const corpusIngestCollectionVar = "CORPUS_INGEST_COLLECTION"

// corpusIngestCollectionRef is how the child declares its own read, including the
// default it falls back to outside a creator-run ingest.
const corpusIngestCollectionRef = "${CORPUS_INGEST_COLLECTION:-corpus}"

func readAgentFile(t *testing.T, agent, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(agentDir(t, agent), name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// execToolParameter is the slice of a declared parameter this file needs: how it
// reaches the child's command line, and where its value comes from.
type execToolParameter struct {
	Flag   string `yaml:"flag"`
	Source string `yaml:"source"`
}

// execTool is the slice of an exec tool declaration this file needs: the
// environment it overlays on the child, and the parameters those entries may
// expand.
type execTool struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	Env        []string `yaml:"env"`
	Parameters struct {
		Properties map[string]execToolParameter `yaml:"properties"`
		Required   []string                     `yaml:"required"`
	} `yaml:"parameters"`
}

type execToolDeclarations struct {
	Tools []execTool `yaml:"tools"`
}

func creatorExecTools(t *testing.T) []execTool {
	t.Helper()
	var declarations execToolDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-declarations.yaml"), &declarations)
	return declarations.Tools
}

func creatorExecTool(t *testing.T, name string) execTool {
	t.Helper()
	for _, tool := range creatorExecTools(t) {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("the creator declares no %s tool", name)
	return execTool{}
}

// The collection the creator resolves, counts, and hands the child must be one
// value. It reaches the child as an environment overlay rather than a flag,
// because the agent binary takes no collection option; the variable name is
// therefore the whole seam, and nothing else checks that the two sides spell it
// the same way.
func TestIngestCollectionIsOneValueForChildAndCreator(t *testing.T) {
	child := readAgentFile(t, "corpus-ingest", "corpus-rest.yaml")
	if !strings.Contains(child, corpusIngestCollectionRef) {
		t.Errorf("the corpus-ingest child does not read %s for the collection it writes",
			corpusIngestCollectionRef)
	}

	tool := creatorExecTool(t, "run_corpus_ingest")
	var overlay string
	for _, entry := range tool.Env {
		key, value, found := strings.Cut(entry, "=")
		if found && key == corpusIngestCollectionVar {
			overlay = value
		}
	}
	if overlay == "" {
		t.Fatalf("run_corpus_ingest sets no %s, so the child writes the creator's own collection whatever the request asked for",
			corpusIngestCollectionVar)
	}
	if overlay != "{{ params.collection }}" {
		t.Errorf("run_corpus_ingest sets %s to %q; the request's collection is what the child must be given",
			corpusIngestCollectionVar, overlay)
	}

	// The creator resolves and counts the same value it hands the child. Sending
	// anything else here is the defect the refusal used to guard against: a delta
	// measured over a collection the child never wrote.
	creator := readAgentFile(t, "creator", "rest.yaml")
	if !strings.Contains(creator, `name: "{{ params.collection }}"`) {
		t.Error("resolve_ingest_collection does not resolve the requested collection, so the counts and the child address different ones")
	}
	if strings.Contains(creator, "name: "+corpusIngestCollectionRef) {
		t.Error("the creator still resolves its own configured collection; a request naming another would be counted in the wrong place")
	}
}

// An env entry may expand only parameters the tool declares -- agent-core
// rejects the declaration otherwise, which turns a typo into a boot failure
// rather than a silent empty value.
func TestExecEnvEntriesExpandOnlyDeclaredParameters(t *testing.T) {
	for _, tool := range creatorExecTools(t) {
		for _, entry := range tool.Env {
			_, value, found := strings.Cut(entry, "=")
			if !found {
				t.Errorf("%s declares env entry %q, which is not KEY=VALUE", tool.Name, entry)
				continue
			}
			for _, name := range envTemplateNames(value) {
				if _, ok := tool.Parameters.Properties[name]; !ok {
					t.Errorf("%s expands {{ params.%s }} but declares no such parameter", tool.Name, name)
				}
			}
		}
	}
}

// envTemplateNames returns the parameter names an env value expands. It matches
// agent-core's own token shape, including the spaces: "{{params.x}}" is not a
// token there and must not look like one here.
func envTemplateNames(value string) []string {
	var names []string
	for rest := value; ; {
		open := strings.Index(rest, "{{ params.")
		if open < 0 {
			return names
		}
		rest = rest[open+len("{{ params."):]
		end := strings.Index(rest, " }}")
		if end < 0 {
			return names
		}
		names = append(names, rest[:end])
		rest = rest[end+len(" }}"):]
	}
}

// The child is told its collection through the environment, so the parameter
// carrying it needs no flag -- the binary has none. It still needs a source, or
// dispatch reads it from the adjacent previous result, which by then is a
// document count.
func TestChildRunTakesTheRequestedCollectionFromCommandState(t *testing.T) {
	tool := creatorExecTool(t, "run_corpus_ingest")

	collection, ok := tool.Parameters.Properties["collection"]
	if !ok {
		t.Fatal("run_corpus_ingest declares no collection parameter, so its env entry cannot expand")
	}
	if collection.Source != "$from(resolve_ingest_collection).carried.collection" {
		t.Errorf("collection is sourced from %q; it must ride the resolve word's carry set, which is the collection actually resolved",
			collection.Source)
	}
	if collection.Flag != "" {
		t.Errorf("collection declares flag %q; the agent binary takes no collection option and the value travels as environment",
			collection.Flag)
	}

	var required bool
	for _, name := range tool.Parameters.Required {
		if name == "collection" {
			required = true
		}
	}
	if !required {
		t.Error("collection is optional, so a run missing it would reach the child and fail on an empty env expansion rather than at dispatch")
	}
}

// With the request's collection honoured there is nothing to refuse. The leg
// runs resolve -> count -> child, and a judge between them would be comparing a
// value against itself.
func TestCreatorNoLongerRefusesARequestedCollection(t *testing.T) {
	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-machine.yaml"), &machine)

	// A resolved collection continues into the leg rather than into a judgement
	// of the collection itself. What follows is the family judge (GH-205), which
	// tests a different value; this asserts only that nothing tests the
	// collection against the deployment's own.
	var resolved bool
	for _, tr := range machine.Transitions {
		if tr.State == "ResolvingIngested" && tr.Signal == "CollectionResolved" {
			resolved = tr.Next != "" && tr.Next != "CollectionRefused"
		}
		if tr.State == "JudgingCollection" || tr.Next == "CollectionRefused" {
			t.Errorf("the collection refusal leg survives: %s on %s -> %s", tr.State, tr.Signal, tr.Next)
		}
	}
	if !resolved {
		t.Error("a resolved collection does not continue into the ingest leg")
	}

	for _, state := range machine.TerminalStates {
		if state == "CollectionRefused" {
			t.Error("CollectionRefused is still terminal, so the response mapping for a refusal outlives the refusal")
		}
	}
	for _, signal := range machine.Signals {
		if signal.Name == "CollectionMatched" || signal.Name == "CollectionMismatched" {
			t.Errorf("%s is still declared; no word emits it", signal.Name)
		}
	}

	creator := readAgentFile(t, "creator", "rest.yaml")
	if strings.Contains(creator, "collection_not_configured") {
		t.Error("the creator still declares a collection_not_configured outcome no terminal reaches")
	}
	declarations := readAgentFile(t, "creator", "request-declarations.yaml")
	if strings.Contains(declarations, "requested_collection_matches") {
		t.Error("the collection judge is still declared; nothing dispatches it")
	}
}

// The caller reads where its documents went rather than assuming the collection
// it named was honoured. The name comes from Chroma's echo, so a request that
// resolved to something else still reports the truth.
func TestCreatorEchoesTheCollectionItIngestedInto(t *testing.T) {
	creator := readAgentFile(t, "creator", "rest.yaml")
	if !strings.Contains(creator, "collection_name: $.name") {
		t.Error("resolve_ingest_collection does not output the name Chroma resolved")
	}

	// Asserting the declaration contained the selector is what let GH-198 ship:
	// the selector was there and resolved to nothing, so the response carried its
	// own text as the collection name. What matters is that the terminal word on
	// the branch actually holds the value the body selects.
	var rest terminalResponseRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)

	terminals := map[string]terminalResponse{}
	for _, server := range rest.Rest.Servers {
		for _, endpoint := range server.Endpoints {
			if endpoint.Path != "/api/v1/ingest" {
				continue
			}
			for name, response := range endpoint.MachineRequest.Response.TerminalStates {
				terminals[name] = response
			}
		}
	}
	if len(terminals) == 0 {
		t.Fatal("the creator declares no /api/v1/ingest terminal responses")
	}
	if _, ok := terminals["CollectionRefused"]; ok {
		t.Error("the ingest endpoint still maps a CollectionRefused response for a terminal the machine no longer declares")
	}

	response, ok := terminals["Ingested"]
	if !ok {
		t.Fatal("the ingest endpoint declares no Ingested terminal")
	}
	selector, ok := response.Body["collection"].(string)
	if !ok || selector == "" {
		t.Fatal("Ingested carries no collection, so a caller cannot tell where its documents landed")
	}
	if !strings.HasPrefix(selector, "$.") {
		t.Errorf("Ingested maps collection to %q, which the runtime returns verbatim", selector)
	}

	// The Ingested branch reaches its value through a word declared for it. The
	// judge that used to be terminal there emits only the predicate fields, so
	// without this word the collection has nowhere to ride.
	machine := readAgentFile(t, "creator", "request-machine.yaml")
	if !strings.Contains(machine, "action: report_ingest_outcome") {
		t.Error("no word puts the collection into the terminal word's output on the Ingested branch")
	}
}

// corpusEmbeddingModelRef is the one reference naming the family an ingest
// embeds at. The child reads it for its own embedding and the creator judges the
// request against it; two literals that happened to agree is the defect the
// collection seam already taught (GH-192).
const corpusEmbeddingModelRef = "${CORPUS_EMBEDDING_MODEL:-qwen3-embedding:8b}"

// The one field in the intent naming an embedding family reached the
// orchestrator's params, the creator's machine request, and then nothing at all:
// no word read it. srd002 R3.3 excludes a source whose reported identity differs
// from the query vector's, so an intent expecting the other family got a 200 and
// documents no turn retrieves (GH-205).
func TestEmbeddingModelReachesTheCreatorOnTheWire(t *testing.T) {
	orchestrator := readAgentFile(t, "provisioning-workflow-orchestrator", "rest.yaml")
	if !strings.Contains(orchestrator, `embedding_model: "{{ params.embedding_model }}"`) {
		t.Error("the orchestrator does not send embedding_model to the creator; the intent's family never leaves it")
	}
	creator := readAgentFile(t, "creator", "rest.yaml")
	if !strings.Contains(creator, "embedding_model: $.embedding_model") {
		t.Error("the creator does not thread embedding_model into its machine request")
	}

	// Threading it is not the point; a word has to read it. Every hop above
	// already passed while the field reached no word at all.
	if !strings.Contains(creator, "carry_forward: [collection, directory, embedding_model]") {
		t.Error("the requested family is not carried past the resolve word, so the judge cannot reach it")
	}
	declarations := readAgentFile(t, "creator", "request-declarations.yaml")
	if !strings.Contains(declarations, "left: $.carried.embedding_model") {
		t.Error("no word reads the requested embedding family; the field is accepted at every hop and consumed by none")
	}
}

// The judge compares against the same reference the child embeds at, not a
// literal of its own. The deployment value is a literal operand: an operand
// starting with neither $. nor $from( is returned as written, and environment
// references expand over the declarations file before it is parsed.
func TestFamilyJudgeComparesTheDeploymentReference(t *testing.T) {
	declarations := readAgentFile(t, "creator", "request-declarations.yaml")
	for _, want := range []string{
		"name: requested_family_matches",
		"left: $.carried.embedding_model",
		"right: " + corpusEmbeddingModelRef,
		"operand_type: string",
		"satisfied: FamilyMatched",
		"unsatisfied: FamilyMismatched",
	} {
		if !strings.Contains(declarations, want) {
			t.Errorf("the family judge does not declare %q", want)
		}
	}

	// The child embeds at the reference the judge compares against. If these
	// drift, the creator refuses requests naming the family the child actually
	// uses, and admits ones it does not.
	child := readAgentFile(t, "corpus-ingest", "corpus-rest.yaml")
	if !strings.Contains(child, corpusEmbeddingModelRef) {
		t.Errorf("the corpus-ingest child does not read %s, so the judge compares against a family nothing embeds at",
			corpusEmbeddingModelRef)
	}
}

// A family this deployment does not ingest at is refused before the child runs.
// Refusing after would reject an ingest that had already written documents no
// turn can retrieve, which is the outcome the judge exists to prevent.
func TestCreatorRefusesAnEmbeddingFamilyItDoesNotIngestAt(t *testing.T) {
	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-machine.yaml"), &machine)

	var refused, matched, failed bool
	for _, tr := range machine.Transitions {
		if tr.State != "JudgingFamily" {
			continue
		}
		switch tr.Signal {
		case "FamilyMismatched":
			refused = tr.Next == "FamilyRefused"
		case "FamilyMatched":
			matched = tr.Next == "CountingBeforeIngest" && tr.Action == "count_before_ingest"
		case "CommandError":
			failed = tr.Next == "IngestFailed"
		}
		if tr.Action == "run_corpus_ingest" {
			t.Error("the family judge runs the child; a refusal would come too late")
		}
	}
	if !refused {
		t.Error("a mismatched family does not reach FamilyRefused; the ingest would run anyway")
	}
	if !matched {
		t.Error("a matched family does not continue to the pre-run count")
	}
	if !failed {
		// An unresolvable operand is a CommandError, not a mismatch: a broken
		// read must not be reported as the caller expecting the wrong family.
		t.Error("JudgingFamily has no path to IngestFailed; a failed comparison would fall through")
	}

	var terminal bool
	for _, state := range machine.TerminalStates {
		if state == "FamilyRefused" {
			terminal = true
		}
	}
	if !terminal {
		t.Error("FamilyRefused is not declared terminal; its response would never be mapped")
	}

	// A shortfall and a mismatched family are different faults, and one error
	// code for both tells an operator reading a trace nothing (srd009 R1.6).
	creator := readAgentFile(t, "creator", "rest.yaml")
	for _, want := range []string{"embedding_family_mismatch", "ingest_shortfall"} {
		if !strings.Contains(creator, want) {
			t.Errorf("the creator declares no %q outcome", want)
		}
	}
}

// Both fields are required for the same shape of reason, reached differently.
// An empty collection fails the child's env expansion at dispatch; an absent
// family cannot be compared at all, because a predicate operand that does not
// resolve is a CommandError rather than a false comparison (agent-core srd041
// R4.2). Neither can be defaulted at the point of use, so both are caught at
// intake.
func TestCreatorIngestRequiresTheCollectionAndFamily(t *testing.T) {
	creator := readAgentFile(t, "creator", "rest.yaml")
	if !strings.Contains(creator, "required: [directory, collection, embedding_model]") {
		t.Error("the ingest endpoint does not require both the collection and the family; one would fail past intake instead of at it")
	}
	orchestrator := readAgentFile(t, "provisioning-workflow-orchestrator", "rest.yaml")
	if !strings.Contains(orchestrator, "required: [values, collection, directory, embedding_model]") {
		t.Error("the provision intent does not require both, so a request could reach the creator without one")
	}
}
