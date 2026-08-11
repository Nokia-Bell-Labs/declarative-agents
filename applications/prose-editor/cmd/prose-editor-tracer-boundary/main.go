// Copyright (c) 2026 Nokia. All rights reserved.

// prose-editor-tracer-boundary is the deterministic Release 00.1 boundary.
// The shipped agent machines own sequencing; this process only records and
// materializes the boundary operation it is asked to perform.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	workspaceEnv = "PROSE_TRACER_WORKSPACE"
	fixturesEnv  = "PROSE_TRACER_FIXTURES"
	scenarioEnv  = "PROSE_TRACER_SCENARIO"
	sessionEnv   = "PROSE_TRACER_SESSION"
	faultEnv     = "PROSE_TRACER_FAULT"
)

type fixtureSuite struct {
	Source struct {
		Repository string `yaml:"repository"`
		Path       string `yaml:"path"`
		Commit     string `yaml:"commit"`
		File       string `yaml:"file"`
	} `yaml:"source"`
	Scenarios []scenario `yaml:"scenarios"`
}

type scenario struct {
	Name             string   `yaml:"name"`
	SagaID           string   `yaml:"saga_id"`
	EditorResponses  []string `yaml:"editor_responses"`
	CriticResponses  []string `yaml:"critic_responses"`
	ExpectedTerminal string   `yaml:"expected_terminal"`
}

type editorFixture struct {
	Content      string   `yaml:"content"`
	RetrievalIDs []string `yaml:"retrieval_ids"`
}

type criticFixture struct {
	Verdict              string                 `yaml:"verdict"`
	ResponsibleStage     string                 `yaml:"responsible_stage"`
	OriginalContentHash  string                 `yaml:"original_content_hash"`
	CandidateContentHash string                 `yaml:"candidate_content_hash"`
	Feedback             string                 `yaml:"feedback"`
	Findings             []criticFixtureFinding `yaml:"findings"`
}

type criticFixtureFinding struct {
	Category string `yaml:"category"`
	Status   string `yaml:"status"`
}

type criticEvaluation struct {
	Verdict              string          `json:"verdict"`
	ResponsibleStage     string          `json:"responsible_stage"`
	OriginalContentHash  string          `json:"original_content_hash"`
	CandidateContentHash string          `json:"candidate_content_hash"`
	Findings             []criticFinding `json:"findings"`
	Feedback             string          `json:"feedback"`
}

type criticFinding struct {
	Category string `json:"category"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
}

type structureCorpus struct {
	Records []struct {
		ID             string  `yaml:"id"`
		Guidance       string  `yaml:"guidance"`
		Distance       float64 `yaml:"distance"`
		EmbeddingModel string  `yaml:"embedding_model"`
		Source         struct {
			Repository string `yaml:"repository"`
			Path       string `yaml:"path"`
			Commit     string `yaml:"commit"`
			ChunkID    string `yaml:"chunk_id"`
		} `yaml:"source"`
	} `yaml:"records"`
}

type embeddingFixture struct {
	Model  string    `yaml:"model"`
	Vector []float64 `yaml:"vector"`
}

type artifact struct {
	ID        string   `json:"id"`
	Stage     string   `json:"stage"`
	Attempt   int      `json:"attempt"`
	Status    string   `json:"status"`
	Path      string   `json:"path"`
	SHA256    string   `json:"sha256"`
	Parents   []string `json:"parents,omitempty"`
	Producer  string   `json:"producer"`
	Retrieval []string `json:"retrieval_ids,omitempty"`
}

type manifest struct {
	SchemaVersion  string              `json:"schema_version"`
	SagaID         string              `json:"saga_id"`
	Revision       int                 `json:"revision"`
	Terminal       string              `json:"terminal_state,omitempty"`
	Source         sourceIdentity      `json:"source"`
	Artifacts      []artifact          `json:"artifacts"`
	Events         []string            `json:"events"`
	Applied        map[string]bool     `json:"applied"`
	Selected       map[string]string   `json:"selected_lineage"`
	ActionCounts   map[string]int      `json:"action_counts"`
	LastCritic     map[string]any      `json:"last_critic,omitempty"`
	BoundaryPolicy map[string][]string `json:"boundary_policy"`
}

type sourceIdentity struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Commit     string `json:"commit"`
	SHA256     string `json:"sha256"`
}

type receipt struct {
	Session    string `json:"session"`
	Sequence   int    `json:"sequence"`
	Operation  string `json:"operation"`
	Occurrence int    `json:"occurrence"`
	Status     string `json:"status"`
	Replay     bool   `json:"replay,omitempty"`
	InputHash  string `json:"input_hash,omitempty"`
	OutputHash string `json:"output_hash,omitempty"`
}

type boundary struct {
	workspace string
	fixtures  string
	session   string
	suite     fixtureSuite
	scenario  scenario
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: prose-editor-tracer-boundary <operation>")
	}
	b, err := loadBoundary()
	if err != nil {
		fatalf("%v", err)
	}
	if os.Args[1] == "serve" {
		if len(os.Args) != 5 {
			fatalf("serve requires model, RAG, and readiness file descriptors")
		}
		listeners, err := inheritedListeners(os.Args[2:4])
		if err != nil {
			fatalf("%v", err)
		}
		readiness, err := inheritedFile(os.Args[4], "prose-tracer-readiness")
		if err != nil {
			closeListeners(listeners)
			fatalf("%v", err)
		}
		if err := b.serve(listeners, readiness); err != nil {
			fatalf("%v", err)
		}
		return
	}
	if err := b.run(os.Args[1]); err != nil {
		fatalf("%v", err)
	}
}

// loadBoundary takes its per-run configuration from the environment because this
// binary is a grandchild, not a command a person runs. The tracer integration
// launches the agent, and the agent runs this binary as a declared exec tool whose
// argv is fixed in YAML -- args: [capture-source] and its siblings. The workspace,
// fixture, session, scenario, and fault values belong to one integration run, so
// putting them in argv would mean writing run-specific values into declarations
// that do not own them, expanded from the environment anyway. The environment is
// the channel that reaches a grandchild without that (GH-1481).
func loadBoundary() (*boundary, error) {
	b := &boundary{
		//nolint:forbidigo // Per-run values the integration passes through the agent to this exec-tool grandchild; see the note above.
		workspace: strings.TrimSpace(os.Getenv(workspaceEnv)),
		//nolint:forbidigo // Per-run fixture root from the same parent contract.
		fixtures: strings.TrimSpace(os.Getenv(fixturesEnv)),
		//nolint:forbidigo // Per-run session identifier from the same parent contract.
		session: strings.TrimSpace(os.Getenv(sessionEnv)),
	}
	if b.workspace == "" || b.fixtures == "" || b.session == "" {
		return nil, fmt.Errorf("%s, %s, and %s are required", workspaceEnv, fixturesEnv, sessionEnv)
	}
	data, err := os.ReadFile(filepath.Join(b.fixtures, "scenarios.yaml"))
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &b.suite); err != nil {
		return nil, err
	}
	//nolint:forbidigo // Selects the scenario for this run from the same parent contract as the fields above.
	name := strings.TrimSpace(os.Getenv(scenarioEnv))
	for _, candidate := range b.suite.Scenarios {
		if candidate.Name == name {
			b.scenario = candidate
		}
	}
	if b.scenario.Name == "" {
		return nil, fmt.Errorf("unknown tracer scenario %q", name)
	}
	return b, os.MkdirAll(filepath.Join(b.workspace, ".tracer"), 0o755)
}

func (b *boundary) run(operation string) error {
	state, err := b.loadManifest()
	if err != nil {
		return err
	}
	occurrence := 0
	key := operation
	var manifestRevision manifestRevisionInput
	var childRequest childRequestInput
	if operation == "append-manifest-revision" {
		manifestRevision, err = parseManifestRevisionInput(os.Args)
		if err != nil {
			return err
		}
		occurrence = manifestRevision.Occurrence
		key = operation + ":" + manifestRevision.Event
	} else if operation == "persist-child-request" {
		childRequest, err = parseChildRequestInput(os.Args)
		if err != nil {
			return err
		}
		occurrence = childRequest.Occurrence
		key = operation + ":" + childRequest.Path
	} else {
		occurrence = b.nextSessionOccurrence(operation)
		key = operation + ":" + strconv.Itoa(occurrence)
	}
	if b.faults(operation, occurrence) {
		_ = b.record(receipt{
			Session: b.session, Operation: operation, Occurrence: occurrence, Status: "injected_failure",
		})
		return fmt.Errorf("injected %s boundary failure at occurrence %d", operation, occurrence)
	}
	replay := state.Applied[key]
	var output string
	var inputHash, outputHash string
	switch operation {
	case "capture-source":
		output, outputHash, err = b.captureSource(&state, key, replay)
	case "write-original":
		output, outputHash, err = b.writeOriginal(&state, key, replay)
	case "append-manifest-revision":
		output, err = b.appendManifest(&state, manifestRevision, key, replay)
	case "write-structure-attempt":
		var input []byte
		if len(os.Args) == 3 {
			input = []byte(os.Args[2])
		} else {
			input, err = io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		}
		inputHash = digest(input)
		if err == nil {
			output, outputHash, err = b.writeStructure(&state, occurrence, key, input, replay)
		}
	case "write-critique-attempt":
		var input []byte
		input, err = io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		inputHash = digest(input)
		if err == nil {
			output, outputHash, err = b.writeCritique(&state, occurrence, key, input, replay)
		}
	case "persist-child-request":
		output, outputHash, err = b.persistChildRequest(childRequest)
	case "materialize-final-chain":
		output, outputHash, err = b.materializeFinal(&state, key, replay)
	default:
		err = fmt.Errorf("unknown operation %q", operation)
	}
	if err != nil {
		_ = b.record(receipt{
			Session: b.session, Operation: operation, Occurrence: occurrence, Status: "error",
			Replay: replay, InputHash: inputHash,
		})
		return err
	}
	if !replay {
		state.Applied[key] = true
		if err := b.saveManifest(state); err != nil {
			return err
		}
	}
	if err := b.record(receipt{
		Session: b.session, Operation: operation, Occurrence: occurrence, Status: "ok",
		Replay: replay, InputHash: inputHash, OutputHash: outputHash,
	}); err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

type childRequestInput struct {
	Path       string
	Occurrence int
}

func parseChildRequestInput(args []string) (childRequestInput, error) {
	if len(args) != 4 {
		return childRequestInput{}, fmt.Errorf("persist-child-request requires path and occurrence arguments")
	}
	clean := filepath.ToSlash(filepath.Clean(args[2]))
	if !strings.HasPrefix(clean, ".tracer/requests/") || filepath.IsAbs(args[2]) {
		return childRequestInput{}, fmt.Errorf("persist-child-request path must be under .tracer/requests")
	}
	occurrence, err := strconv.Atoi(args[3])
	if err != nil || occurrence < 1 {
		return childRequestInput{}, fmt.Errorf("persist-child-request occurrence must be positive")
	}
	return childRequestInput{Path: clean, Occurrence: occurrence}, nil
}

func (b *boundary) persistChildRequest(input childRequestInput) (string, string, error) {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		return "", "", err
	}
	if !json.Valid(data) {
		return "", "", fmt.Errorf("persist-child-request input must be valid JSON")
	}
	path := filepath.Join(b.workspace, filepath.FromSlash(input.Path))
	if err := writeProjection(path, data); err != nil {
		return "", "", err
	}
	return path, digest(data), nil
}

func (b *boundary) captureSource(state *manifest, key string, replay bool) (string, string, error) {
	data, err := os.ReadFile(filepath.Join(b.fixtures, filepath.FromSlash(b.suite.Source.File)))
	if err != nil {
		return "", "", err
	}
	sum := digest(data)
	path := filepath.Join(b.workspace, ".tracer", "captured-source.md")
	if err := writeImmutable(path, data); err != nil {
		return "", "", err
	}
	if !replay {
		state.Source = sourceIdentity{
			Repository: b.suite.Source.Repository, Path: b.suite.Source.Path,
			Commit: b.suite.Source.Commit, SHA256: sum,
		}
		state.Events = append(state.Events, "source_captured")
	}
	return `{"captured":true}`, sum, nil
}

func (b *boundary) writeOriginal(state *manifest, key string, replay bool) (string, string, error) {
	data, err := os.ReadFile(filepath.Join(b.workspace, ".tracer", "captured-source.md"))
	if err != nil {
		return "", "", err
	}
	if err := writeImmutable(filepath.Join(b.workspace, "00-original.md"), data); err != nil {
		return "", "", err
	}
	sum := digest(data)
	if !replay {
		id := artifactID("original", 1, sum)
		state.Artifacts = append(state.Artifacts, artifact{
			ID: id, Stage: "original", Attempt: 1, Status: "captured",
			Path: "00-original.md", SHA256: sum, Producer: "workflow-orchestrator",
		})
		state.Selected["original"] = id
		state.Events = append(state.Events, "original_written")
	}
	return `{"written":true}`, sum, nil
}

type manifestRevisionInput struct {
	Event          string
	Terminal       string
	Occurrence     int
	ContextAttempt int
}

func parseManifestRevisionInput(args []string) (manifestRevisionInput, error) {
	if len(args) != 6 {
		return manifestRevisionInput{}, fmt.Errorf(
			"append-manifest-revision requires event, terminal state, occurrence, and context attempt arguments",
		)
	}
	occurrence, err := strconv.Atoi(args[4])
	if err != nil || occurrence < 1 {
		return manifestRevisionInput{}, fmt.Errorf("append-manifest-revision occurrence must be positive")
	}
	terminal := args[3]
	if terminal == "none" {
		terminal = ""
	}
	if terminal != "" && terminal != "LocallyFinalized" && terminal != "KeptOriginal" {
		return manifestRevisionInput{}, fmt.Errorf("unsupported manifest terminal state %q", terminal)
	}
	if strings.TrimSpace(args[2]) == "" {
		return manifestRevisionInput{}, fmt.Errorf("append-manifest-revision event is required")
	}
	contextAttempt, err := strconv.Atoi(args[5])
	if err != nil || contextAttempt < 0 {
		return manifestRevisionInput{}, fmt.Errorf("append-manifest-revision context attempt must be non-negative")
	}
	return manifestRevisionInput{
		Event: args[2], Terminal: terminal, Occurrence: occurrence, ContextAttempt: contextAttempt,
	}, nil
}

func (b *boundary) appendManifest(
	state *manifest,
	revision manifestRevisionInput,
	key string,
	replay bool,
) (string, error) {
	if !replay {
		state.Revision++
		state.Events = append(state.Events, revision.Event)
		if revision.Event == "retry_recorded" {
			if id := state.Selected["structure"]; id != "" {
				for index := range state.Artifacts {
					if state.Artifacts[index].ID == id {
						state.Artifacts[index].Status = "superseded"
					}
				}
			}
		}
		if revision.Terminal != "" {
			state.Terminal = revision.Terminal
		}
		if revision.Terminal == "KeptOriginal" {
			state.Selected["structure"] = ""
			state.Selected["critique"] = ""
			state.Selected["final"] = ""
		}
	}
	if !replay {
		if err := b.saveManifestHistory(*state); err != nil {
			return "", err
		}
	}
	context, err := b.manifestContext(*state, revision.ContextAttempt)
	if err != nil {
		return "", err
	}
	return string(context), nil
}

func (b *boundary) manifestContext(state manifest, structureAttempt int) ([]byte, error) {
	original, err := os.ReadFile(filepath.Join(b.workspace, "00-original.md"))
	if err != nil {
		return nil, err
	}
	context := map[string]any{
		"original_content": string(original), "original_artifact_id": state.Selected["original"],
		"original_content_hash": state.Source.SHA256, "saga_id": state.SagaID,
	}
	if structureAttempt > 0 {
		structure, ok := artifactByStageAttempt(state, "structure", structureAttempt)
		if !ok {
			return nil, fmt.Errorf("manifest context requires structure attempt %d", structureAttempt)
		}
		candidate, err := os.ReadFile(filepath.Join(b.workspace, filepath.FromSlash(structure.Path)))
		if err != nil {
			return nil, err
		}
		context["candidate_content"] = string(candidate)
		context["candidate_artifact_id"] = structure.ID
		context["candidate_content_hash"] = structure.SHA256
		context["candidate_stage"] = "structure"
		context["attempt"] = structure.Attempt
	}
	return json.Marshal(context)
}

func (b *boundary) writeStructure(state *manifest, occurrence int, key string, input []byte, replay bool) (string, string, error) {
	attempt := occurrence
	if attempt > len(b.scenario.EditorResponses) {
		return "", "", errors.New("structure attempt exceeds fixture roster")
	}
	var fixture editorFixture
	if err := readYAML(filepath.Join(b.fixtures, b.scenario.EditorResponses[attempt-1]), &fixture); err != nil {
		return "", "", err
	}
	if string(input) != fixture.Content {
		return "", "", errors.New("structure child output differs from deterministic model fixture")
	}
	sum := digest(input)
	relative := filepath.ToSlash(filepath.Join("attempts", "structure", fmt.Sprintf("%04d-%s.md", attempt, sum)))
	if err := writeImmutable(filepath.Join(b.workspace, filepath.FromSlash(relative)), input); err != nil {
		return "", "", err
	}
	if !replay {
		id := artifactID("structure", attempt, sum)
		state.Artifacts = append(state.Artifacts, artifact{
			ID: id, Stage: "structure", Attempt: attempt, Status: "candidate", Path: relative,
			SHA256: sum, Parents: []string{state.Selected["original"]},
			Producer: "specialist-editor:structure", Retrieval: fixture.RetrievalIDs,
		})
		state.Selected["structure"] = id
		state.Events = append(state.Events, fmt.Sprintf("structure_attempt_%d_written", attempt))
	} else {
		want := artifact{
			ID: artifactID("structure", attempt, sum), Stage: "structure", Attempt: attempt,
			Path: relative, SHA256: sum, Parents: []string{state.Selected["original"]},
			Producer: "specialist-editor:structure", Retrieval: fixture.RetrievalIDs,
		}
		if err := validateReplayArtifact(*state, want); err != nil {
			return "", "", err
		}
	}
	return `{"written":true}`, sum, nil
}

func (b *boundary) writeCritique(state *manifest, occurrence int, key string, input []byte, replay bool) (string, string, error) {
	attempt := occurrence
	if attempt > len(b.scenario.CriticResponses) {
		return "", "", errors.New("critic attempt exceeds fixture roster")
	}
	evaluation, err := decodeCriticEvaluation(input)
	if err != nil {
		return "", "", fmt.Errorf("decode critic child output: %w", err)
	}
	structure, ok := selectedArtifact(*state, "structure")
	if replay {
		structure, ok = artifactByStageAttempt(*state, "structure", attempt)
	}
	if !ok {
		return "", "", fmt.Errorf("critique requires structure attempt %d", attempt)
	}
	if err := b.validateCriticEvaluation(*state, structure, evaluation); err != nil {
		return "", "", fmt.Errorf("invalid critic child output: %w", err)
	}
	var fixture criticFixture
	if err := readYAML(filepath.Join(b.fixtures, b.scenario.CriticResponses[attempt-1]), &fixture); err != nil {
		return "", "", err
	}
	if evaluation.Verdict != fixture.Verdict {
		return "", "", errors.New("critic child verdict differs from deterministic model fixture")
	}
	sum := digest(input)
	relative := filepath.ToSlash(filepath.Join("attempts", "critique", fmt.Sprintf("%04d-%s.yaml", attempt, sum)))
	if err := writeImmutable(filepath.Join(b.workspace, filepath.FromSlash(relative)), input); err != nil {
		return "", "", err
	}
	structureID := structure.ID
	if !replay {
		id := artifactID("critique", attempt, sum)
		state.Artifacts = append(state.Artifacts, artifact{
			ID: id, Stage: "critique", Attempt: attempt, Status: evaluation.Verdict, Path: relative,
			SHA256: sum, Parents: []string{state.Selected["original"], structureID},
			Producer: "voice-critic",
		})
		state.Selected["critique"] = id
		state.LastCritic = criticEvaluationMap(evaluation)
		state.Events = append(state.Events, fmt.Sprintf("critique_attempt_%d_written", attempt))
	} else {
		want := artifact{
			ID: artifactID("critique", attempt, sum), Stage: "critique", Attempt: attempt,
			Status: evaluation.Verdict, Path: relative, SHA256: sum,
			Parents: []string{state.Selected["original"], structureID}, Producer: "voice-critic",
		}
		if err := validateReplayArtifact(*state, want); err != nil {
			return "", "", err
		}
	}
	return `{"written":true}`, sum, nil
}

func (b *boundary) materializeFinal(state *manifest, key string, replay bool) (string, string, error) {
	structure, ok := selectedArtifact(*state, "structure")
	if !ok {
		return "", "", errors.New("finalization requires selected structure")
	}
	critique, ok := selectedArtifact(*state, "critique")
	if !ok || critique.Status != "pass" {
		return "", "", errors.New("finalization requires selected passed critique")
	}
	if !equalStrings(critique.Parents, []string{state.Selected["original"], structure.ID}) {
		return "", "", errors.New("selected critique does not bind the selected original and structure")
	}
	structureBytes, err := os.ReadFile(filepath.Join(b.workspace, filepath.FromSlash(structure.Path)))
	if err != nil {
		return "", "", err
	}
	if digest(structureBytes) != structure.SHA256 {
		return "", "", errors.New("selected structure bytes differ from recorded hash")
	}
	critiqueBytes, err := os.ReadFile(filepath.Join(b.workspace, filepath.FromSlash(critique.Path)))
	if err != nil {
		return "", "", err
	}
	if digest(critiqueBytes) != critique.SHA256 {
		return "", "", errors.New("selected critique bytes differ from recorded hash")
	}
	evaluation, err := decodeCriticEvaluation(critiqueBytes)
	if err != nil {
		return "", "", fmt.Errorf("decode selected critique: %w", err)
	}
	if err := b.validateCriticEvaluation(*state, structure, evaluation); err != nil {
		return "", "", fmt.Errorf("finalization requires a valid critic evaluation: %w", err)
	}
	if evaluation.Verdict != "pass" {
		return "", "", errors.New("finalization requires a passed critic verdict")
	}
	for path, data := range map[string][]byte{
		"10-structure.md": structureBytes, "40-critique.yaml": critiqueBytes, "final.md": structureBytes,
	} {
		if err := writeImmutable(filepath.Join(b.workspace, path), data); err != nil {
			return "", "", err
		}
	}
	if !replay {
		sum := digest(structureBytes)
		id := artifactID("final", 1, sum)
		state.Artifacts = append(state.Artifacts, artifact{
			ID: id, Stage: "final", Attempt: 1, Status: "finalized", Path: "final.md",
			SHA256: sum, Parents: []string{structure.ID, critique.ID}, Producer: "workflow-orchestrator",
		})
		state.Selected["final"] = id
		state.Events = append(state.Events, "final_chain_materialized")
	}
	return `{"materialized":true}`, digest(structureBytes), nil
}

func inheritedListeners(arguments []string) ([]net.Listener, error) {
	if len(arguments) != 2 {
		return nil, errors.New("serve requires model and RAG listener file descriptors")
	}
	listeners := make([]net.Listener, 0, len(arguments))
	for index, argument := range arguments {
		file, err := inheritedFile(argument, fmt.Sprintf("prose-tracer-listener-%d", index))
		if err != nil {
			closeListeners(listeners)
			return nil, err
		}
		listener, err := net.FileListener(file)
		_ = file.Close()
		if err != nil {
			closeListeners(listeners)
			return nil, fmt.Errorf("inherit listener file descriptor %s: %w", argument, err)
		}
		listeners = append(listeners, listener)
	}
	return listeners, nil
}

func inheritedFile(argument, name string) (*os.File, error) {
	fd, err := strconv.Atoi(argument)
	if err != nil || fd < 3 {
		return nil, fmt.Errorf("invalid inherited file descriptor %q", argument)
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		return nil, fmt.Errorf("open inherited file descriptor %d", fd)
	}
	return file, nil
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func (b *boundary) serve(listeners []net.Listener, readiness *os.File) error {
	defer readiness.Close()
	if len(listeners) != 2 {
		closeListeners(listeners)
		return fmt.Errorf("serve requires 2 listeners, got %d", len(listeners))
	}
	handler := http.HandlerFunc(b.handleHTTP)
	servers := make([]*http.Server, 0, 2)
	errs := make(chan error, 2)
	for _, listener := range listeners {
		server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
		servers = append(servers, server)
		go func() { errs <- server.Serve(listener) }()
	}
	if _, err := readiness.Write([]byte{1}); err != nil {
		closeListeners(listeners)
		return fmt.Errorf("signal boundary readiness: %w", err)
	}
	_ = readiness.Close()
	fmt.Println("tracer boundary ready")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-signals:
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	for _, server := range servers {
		_ = server.Close()
	}
	return nil
}

func (b *boundary) handleHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	operation := "http:" + r.URL.Path
	occurrence := b.httpOccurrence(operation)
	if b.faults(operation, occurrence) {
		_ = b.record(receipt{
			Session: b.session, Operation: operation, Occurrence: occurrence, Status: "injected_failure",
			InputHash: digest(body),
		})
		http.Error(w, "injected boundary failure", http.StatusInternalServerError)
		return
	}
	var response any
	switch r.URL.Path {
	case "/health":
		response = map[string]any{"status": "ok"}
	case "/api/embeddings":
		var fixture embeddingFixture
		if err := readYAML(filepath.Join(b.fixtures, "retrieval", "embedding.yaml"), &fixture); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response = map[string]any{"embedding": fixture.Vector, "model": fixture.Model}
	case "/api/v1/rag/query":
		var corpus structureCorpus
		if err := readYAML(filepath.Join(b.fixtures, "retrieval", "structure-corpus.yaml"), &corpus); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(corpus.Records) != 1 {
			http.Error(w, "tracer structure corpus must contain exactly one record", http.StatusInternalServerError)
			return
		}
		record := corpus.Records[0]
		response = map[string]any{
			"ids":       []any{[]string{record.ID}},
			"documents": []any{[]string{record.Guidance}},
			"distances": []any{[]float64{record.Distance}},
			"metadatas": []any{[]map[string]string{{
				"repository": record.Source.Repository, "path": record.Source.Path,
				"commit": record.Source.Commit, "chunk_id": record.Source.ChunkID,
			}}},
			"embedding_model": record.EmbeddingModel,
		}
	case "/api/chat":
		response, err = b.chatResponse(body, occurrence)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
	_ = b.record(receipt{
		Session: b.session, Operation: operation, Occurrence: occurrence, Status: "ok",
		InputHash: digest(body), OutputHash: digest(data),
	})
}

func (b *boundary) chatResponse(body []byte, occurrence int) (any, error) {
	var request struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}
	var content []byte
	switch request.Model {
	case "tracer-editor":
		editorOccurrence := b.httpModelOccurrence("tracer-editor")
		if editorOccurrence > len(b.scenario.EditorResponses) {
			return nil, errors.New("editor model fixture exhausted")
		}
		var fixture editorFixture
		if err := readYAML(filepath.Join(b.fixtures, b.scenario.EditorResponses[editorOccurrence-1]), &fixture); err != nil {
			return nil, err
		}
		state, err := b.loadManifest()
		if err != nil {
			return nil, err
		}
		requestData, err := os.ReadFile(b.childRequestPath("structure", editorOccurrence))
		if err != nil {
			return nil, err
		}
		var childRequest map[string]any
		if err := json.Unmarshal(requestData, &childRequest); err != nil {
			return nil, err
		}
		content, err = json.Marshal(map[string]any{
			"outcome": "candidate", "content": fixture.Content,
			"parent_artifact_id":  childRequest["parent_artifact_id"],
			"parent_content_hash": childRequest["parent_content_hash"],
			"retrieval_provenance": []map[string]any{{
				"id": fixture.RetrievalIDs[0], "embedding_model": "tracer-embedding",
				"source_hash": state.Source.SHA256,
			}},
		})
		if err != nil {
			return nil, err
		}
	case "tracer-critic":
		criticOccurrence := b.httpModelOccurrence("tracer-critic")
		if criticOccurrence > len(b.scenario.CriticResponses) {
			return nil, errors.New("critic model fixture exhausted")
		}
		var fixture criticFixture
		if err := readYAML(filepath.Join(b.fixtures, b.scenario.CriticResponses[criticOccurrence-1]), &fixture); err != nil {
			return nil, err
		}
		requestData, err := os.ReadFile(b.childRequestPath("critic", criticOccurrence))
		if err != nil {
			return nil, err
		}
		var childRequest map[string]any
		if err := json.Unmarshal(requestData, &childRequest); err != nil {
			return nil, err
		}
		findings := make([]map[string]string, 0, len(fixture.Findings))
		for _, finding := range fixture.Findings {
			findings = append(findings, map[string]string{
				"category": finding.Category, "status": finding.Status, "summary": "fixture-backed assessment",
			})
		}
		originalHash := fixture.OriginalContentHash
		if originalHash == "" {
			originalHash, _ = childRequest["original_content_hash"].(string)
		}
		candidateHash := fixture.CandidateContentHash
		if candidateHash == "" {
			candidateHash, _ = childRequest["candidate_content_hash"].(string)
		}
		content, err = json.Marshal(map[string]any{
			"verdict": fixture.Verdict, "responsible_stage": fixture.ResponsibleStage,
			"original_content_hash":  originalHash,
			"candidate_content_hash": candidateHash,
			"findings":               findings, "feedback": fixture.Feedback,
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unexpected model %q", request.Model)
	}
	return map[string]any{
		"message":    map[string]any{"role": "assistant", "content": string(content)},
		"eval_count": 1, "prompt_eval_count": 1,
	}, nil
}

func (b *boundary) childRequestPath(stage string, occurrence int) string {
	return filepath.Join(
		b.workspace, ".tracer", "requests", fmt.Sprintf("%s-%d.json", stage, occurrence),
	)
}

func (b *boundary) loadManifest() (manifest, error) {
	path := filepath.Join(b.workspace, "manifest.yaml")
	var state manifest
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		state = manifest{
			SchemaVersion: "prose-editor.interpreter-trace/v1", SagaID: b.scenario.SagaID,
			Applied: map[string]bool{}, Selected: map[string]string{}, ActionCounts: map[string]int{},
			BoundaryPolicy: map[string][]string{
				"forbidden": {"git", "github_publication", "pangram", "voice_editor", "style_editor", "helm", "kind", "kubectl"},
			},
		}
		return state, nil
	}
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func (b *boundary) saveManifest(state manifest) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeProjection(filepath.Join(b.workspace, "manifest.yaml"), append(data, '\n'))
}

func (b *boundary) saveManifestHistory(state manifest) error {
	if state.Revision == 0 {
		return nil
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeImmutable(
		filepath.Join(b.workspace, "manifest-history", fmt.Sprintf("%04d.yaml", state.Revision)),
		append(data, '\n'),
	)
}

func (b *boundary) nextSessionOccurrence(operation string) int {
	receipts, _ := b.receipts()
	count := 1
	for _, existing := range receipts {
		if existing.Session == b.session && existing.Operation == operation {
			count++
		}
	}
	return count
}

func (b *boundary) httpOccurrence(operation string) int {
	receipts, _ := b.receipts()
	count := 1
	for _, existing := range receipts {
		if existing.Session == b.session && existing.Operation == operation {
			count++
		}
	}
	return count
}

func (b *boundary) httpModelOccurrence(model string) int {
	receipts, _ := b.receipts()
	count := 1
	for _, existing := range receipts {
		if existing.Session == b.session && existing.Operation == "model:"+model {
			count++
		}
	}
	_ = b.record(receipt{
		Session: b.session, Operation: "model:" + model, Occurrence: count, Status: "selected",
	})
	return count
}

// faults reports whether this run injects a fault at this operation occurrence.
// The selector comes from the environment for the same reason the fields in
// loadBoundary do: it is a per-run value the integration passes through the agent.
func (b *boundary) faults(operation string, occurrence int) bool {
	want := operation + ":" + strconv.Itoa(occurrence)
	//nolint:forbidigo // Per-run fault selector from the parent contract described on loadBoundary.
	return strings.TrimSpace(os.Getenv(faultEnv)) == want
}

func (b *boundary) record(value receipt) error {
	receipts, _ := b.receipts()
	value.Sequence = len(receipts) + 1
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	path := filepath.Join(b.workspace, "boundary-receipts.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.Write(append(data, '\n'))
	return err
}

func (b *boundary) receipts() ([]receipt, error) {
	file, err := os.Open(filepath.Join(b.workspace, "boundary-receipts.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var values []receipt
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var value receipt
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, scanner.Err()
}

func decodeCriticEvaluation(data []byte) (criticEvaluation, error) {
	var evaluation criticEvaluation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evaluation); err != nil {
		return evaluation, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return evaluation, errors.New("critic output contains multiple JSON values")
		}
		return evaluation, err
	}
	return evaluation, nil
}

func (b *boundary) validateCriticEvaluation(
	state manifest,
	structure artifact,
	evaluation criticEvaluation,
) error {
	original, ok := selectedArtifact(state, "original")
	if !ok || original.Stage != "original" {
		return errors.New("critic evaluation requires selected original")
	}
	if structure.Stage != "structure" || structure.ID == "" {
		return errors.New("critic evaluation requires selected structure")
	}
	originalBytes, err := os.ReadFile(filepath.Join(b.workspace, filepath.FromSlash(original.Path)))
	if err != nil {
		return fmt.Errorf("read selected original: %w", err)
	}
	candidateBytes, err := os.ReadFile(filepath.Join(b.workspace, filepath.FromSlash(structure.Path)))
	if err != nil {
		return fmt.Errorf("read selected structure: %w", err)
	}
	originalHash := digest(originalBytes)
	candidateHash := digest(candidateBytes)
	if originalHash != original.SHA256 || candidateHash != structure.SHA256 {
		return errors.New("selected artifact bytes differ from recorded hashes")
	}
	if evaluation.OriginalContentHash != originalHash {
		return errors.New("critic original hash does not match selected artifact bytes")
	}
	if evaluation.CandidateContentHash != candidateHash {
		return errors.New("critic candidate hash does not match selected artifact bytes")
	}

	required := map[string]bool{
		"semantic_preservation": false,
		"structural_intent":     false,
		"voice_match":           false,
		"tightening_quality":    false,
		"unsupported_additions": false,
		"anchor_copy_risk":      false,
	}
	if len(evaluation.Findings) != len(required) {
		return fmt.Errorf("critic findings count = %d, want %d", len(evaluation.Findings), len(required))
	}
	rejects := 0
	for _, finding := range evaluation.Findings {
		seen, known := required[finding.Category]
		if !known {
			return fmt.Errorf("unknown critic finding category %q", finding.Category)
		}
		if seen {
			return fmt.Errorf("duplicate critic finding category %q", finding.Category)
		}
		required[finding.Category] = true
		switch finding.Status {
		case "pass":
		case "warn":
		case "reject":
			rejects++
		default:
			return fmt.Errorf("invalid critic finding status %q", finding.Status)
		}
	}
	for category, seen := range required {
		if !seen {
			return fmt.Errorf("missing critic finding category %q", category)
		}
	}

	switch evaluation.Verdict {
	case "pass":
		if evaluation.ResponsibleStage != "" {
			return errors.New("passed critic evaluation must not name a responsible stage")
		}
		for _, finding := range evaluation.Findings {
			if finding.Status != "pass" {
				return errors.New("passed critic evaluation requires all findings to pass")
			}
		}
	case "reject":
		if evaluation.ResponsibleStage != "structure" {
			return errors.New("Release 00.1 rejected evaluation must name structure as responsible stage")
		}
		if rejects == 0 {
			return errors.New("rejected critic evaluation requires at least one rejected finding")
		}
	default:
		return fmt.Errorf("invalid critic verdict %q", evaluation.Verdict)
	}
	return nil
}

func criticEvaluationMap(evaluation criticEvaluation) map[string]any {
	data, _ := json.Marshal(evaluation)
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	return decoded
}

func selectedArtifact(state manifest, stage string) (artifact, bool) {
	id := state.Selected[stage]
	for _, candidate := range state.Artifacts {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return artifact{}, false
}

func artifactByStageAttempt(state manifest, stage string, attempt int) (artifact, bool) {
	for _, candidate := range state.Artifacts {
		if candidate.Stage == stage && candidate.Attempt == attempt {
			return candidate, true
		}
	}
	return artifact{}, false
}

func validateReplayArtifact(state manifest, want artifact) error {
	for _, candidate := range state.Artifacts {
		if candidate.ID != want.ID {
			continue
		}
		if candidate.Stage != want.Stage || candidate.Attempt != want.Attempt ||
			candidate.Path != want.Path || candidate.SHA256 != want.SHA256 ||
			candidate.Producer != want.Producer ||
			!equalStrings(candidate.Parents, want.Parents) ||
			!equalStrings(candidate.Retrieval, want.Retrieval) ||
			(want.Status != "" && candidate.Status != want.Status) {
			return fmt.Errorf("replay artifact %s differs from recorded lineage", want.ID)
		}
		return nil
	}
	return fmt.Errorf("replay artifact %s is not recorded", want.ID)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func readYAML(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, value)
}

func writeImmutable(path string, data []byte) error {
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("immutable path differs: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o444)
}

func writeProjection(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func artifactID(stage string, attempt int, sum string) string {
	return fmt.Sprintf("%s-%04d-%s", stage, attempt, sum[:16])
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
