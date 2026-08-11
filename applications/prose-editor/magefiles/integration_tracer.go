// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
)

const tracerFixtureRoot = "testdata/integration/tracer"

// Integration contains the hermetic application integration gates.
type Integration mg.Namespace

// Tracer executes the shipped Release 00.1 machines through agent-core.
func (Integration) Tracer() error {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return err
	}
	if err := validateTracerProfileBoot(root); err != nil {
		return err
	}
	runtime, err := buildTracerRuntime(root)
	if err != nil {
		return err
	}
	defer runtime.close()

	suite, err := loadInterpreterSuite(root)
	if err != nil {
		return err
	}
	for _, scenario := range suite.Scenarios {
		if scenario.ExpectedTerminal == "failed" {
			if err := runInvalidCriticEvidence(runtime, scenario); err != nil {
				return fmt.Errorf("scenario %s: %w", scenario.Name, err)
			}
			continue
		}
		if err := runInterpreterEvidence(root, runtime, scenario); err != nil {
			return fmt.Errorf("scenario %s: %w", scenario.Name, err)
		}
	}
	if err := runInterpreterFailureEvidence(root, runtime, suite); err != nil {
		return err
	}
	fmt.Println("integration:tracer PASS - agent-core executed workflow-orchestrator, specialist-editor, and voice-critic machines")
	return nil
}

// All is the release aggregate. Release 00.1 owns only the tracer gate.
func (i Integration) All() error {
	return i.Tracer()
}

type interpreterSuite struct {
	SchemaVersion string                `yaml:"schema_version"`
	Scenarios     []interpreterScenario `yaml:"scenarios"`
}

type interpreterScenario struct {
	Name             string   `yaml:"name"`
	SagaID           string   `yaml:"saga_id"`
	EditorResponses  []string `yaml:"editor_responses"`
	CriticResponses  []string `yaml:"critic_responses"`
	ExpectedTerminal string   `yaml:"expected_terminal"`
}

type tracerRuntime struct {
	buildDir      string
	agent         string
	boundary      string
	coreRoot      string
	orchestrator  string
	workflowCWD   string
	fixtures      string
	application   string
	modelListener *net.TCPListener
	ragListener   *net.TCPListener
}

type interpreterRun struct {
	Session string
	Stdout  string
	Stderr  string
	Trace   string
}

type recordedManifest struct {
	SchemaVersion string             `json:"schema_version"`
	SagaID        string             `json:"saga_id"`
	Revision      int                `json:"revision"`
	Terminal      string             `json:"terminal_state"`
	Source        recordedSource     `json:"source"`
	Artifacts     []recordedArtifact `json:"artifacts"`
	Events        []string           `json:"events"`
	Selected      map[string]string  `json:"selected_lineage"`
}

type recordedSource struct {
	SHA256 string `json:"sha256"`
}

type recordedArtifact struct {
	ID       string   `json:"id"`
	Stage    string   `json:"stage"`
	Attempt  int      `json:"attempt"`
	Status   string   `json:"status"`
	Path     string   `json:"path"`
	SHA256   string   `json:"sha256"`
	Parents  []string `json:"parents"`
	Producer string   `json:"producer"`
}

type recordedReceipt struct {
	Session    string `json:"session"`
	Operation  string `json:"operation"`
	Occurrence int    `json:"occurrence"`
	Status     string `json:"status"`
	Replay     bool   `json:"replay"`
}

func loadInterpreterSuite(root string) (interpreterSuite, error) {
	var suite interpreterSuite
	err := readYAML(filepath.Join(root, tracerFixtureRoot, "scenarios.yaml"), &suite)
	if err != nil {
		return suite, err
	}
	required := map[string]string{
		"happy":                              "locally_finalized",
		"reject-then-accept":                 "locally_finalized",
		"retry-exhausted":                    "kept_original",
		"fault-wrong-hashes":                 "failed",
		"fault-duplicate-missing-categories": "failed",
		"fault-pass-with-stage":              "failed",
		"fault-reject-without-stage":         "failed",
		"fault-pass-with-reject-status":      "failed",
		"fault-reject-with-passing-statuses": "failed",
	}
	if suite.SchemaVersion != "prose-editor.tracer-scenarios/v1" || len(suite.Scenarios) != len(required) {
		return suite, errors.New("interpreter tracer fixture roster is not Release 00.1")
	}
	for _, scenario := range suite.Scenarios {
		if terminal, ok := required[scenario.Name]; !ok || terminal != scenario.ExpectedTerminal {
			return suite, fmt.Errorf("unexpected tracer scenario %q with terminal %q", scenario.Name, scenario.ExpectedTerminal)
		}
		delete(required, scenario.Name)
	}
	if len(required) != 0 {
		return suite, fmt.Errorf("tracer fixture roster is missing %v", sortedKeys(required))
	}
	return suite, nil
}

func buildTracerRuntime(root string) (tracerRuntime, error) {
	buildDir, err := os.MkdirTemp("", "prose-editor-interpreter-*")
	if err != nil {
		return tracerRuntime{}, err
	}
	modelListener, err := reserveLoopbackListener()
	if err != nil {
		_ = os.RemoveAll(buildDir)
		return tracerRuntime{}, fmt.Errorf("reserve model boundary listener: %w", err)
	}
	ragListener, err := reserveLoopbackListener()
	if err != nil {
		_ = modelListener.Close()
		_ = os.RemoveAll(buildDir)
		return tracerRuntime{}, fmt.Errorf("reserve RAG boundary listener: %w", err)
	}
	runtime := tracerRuntime{
		buildDir: buildDir, agent: filepath.Join(buildDir, "agent"),
		boundary:     filepath.Join(buildDir, "prose-editor-tracer-boundary"),
		coreRoot:     filepath.Clean(filepath.Join(root, "..", "..", "agent-core")),
		orchestrator: filepath.Join(root, "agents", "workflow-orchestrator", "profile.yaml"),
		workflowCWD:  filepath.Join(root, "agents", "workflow-orchestrator"),
		fixtures:     filepath.Join(root, tracerFixtureRoot), application: root,
		modelListener: modelListener, ragListener: ragListener,
	}
	if err := runBuild(runtime.coreRoot, "go", "build", "-o", runtime.agent, "./cmd/agent"); err != nil {
		runtime.close()
		return tracerRuntime{}, err
	}
	if err := runBuild(root, "go", "build", "-o", runtime.boundary, "./cmd/prose-editor-tracer-boundary"); err != nil {
		runtime.close()
		return tracerRuntime{}, err
	}
	return runtime, nil
}

func reserveLoopbackListener() (*net.TCPListener, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		return nil, err
	}
	return listener, nil
}

func (runtime tracerRuntime) close() {
	if runtime.modelListener != nil {
		_ = runtime.modelListener.Close()
	}
	if runtime.ragListener != nil {
		_ = runtime.ragListener.Close()
	}
	_ = os.RemoveAll(runtime.buildDir)
}

func runBuild(directory, binary string, args ...string) error {
	command := exec.Command(binary, args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", binary, strings.Join(args, " "), err, output)
	}
	return nil
}

func runInterpreterEvidence(root string, runtime tracerRuntime, scenario interpreterScenario) error {
	workspace, err := os.MkdirTemp("", "prose-editor-"+scenario.Name+"-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)

	first, err := runAgentSession(runtime, scenario, workspace, "initial", "")
	if err != nil {
		return err
	}
	if err := validateInterpreterWorkspace(workspace, scenario, first); err != nil {
		return err
	}
	fmt.Printf("interpreter scenario %s: workflow=%s structure_children=%d critic_children=%d\n",
		scenario.Name, expectedWorkflowTerminal(scenario), len(scenario.EditorResponses), len(scenario.CriticResponses))
	manifestBefore, err := manifestSemantics(workspace)
	if err != nil {
		return err
	}
	before, err := immutableArtifactDigest(workspace)
	if err != nil {
		return err
	}

	replay, err := runAgentSession(runtime, scenario, workspace, "replay", "")
	if err != nil {
		return err
	}
	if err := validateInterpreterWorkspace(workspace, scenario, replay); err != nil {
		return err
	}
	after, err := immutableArtifactDigest(workspace)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("terminal interpreter replay changed immutable artifacts")
	}
	manifestAfter, err := manifestSemantics(workspace)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(manifestBefore, manifestAfter) {
		return errors.New("terminal interpreter replay changed manifest semantics")
	}
	receipts, err := loadReceipts(workspace)
	if err != nil {
		return err
	}
	if !sessionHasReplay(receipts, "replay") {
		return errors.New("terminal interpreter replay produced no replay receipts")
	}
	return nil
}

func runInvalidCriticEvidence(runtime tracerRuntime, scenario interpreterScenario) error {
	workspace, err := os.MkdirTemp("", "prose-editor-"+scenario.Name+"-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)

	run, runErr := runAgentSession(runtime, scenario, workspace, "invalid-critic", "")
	if runErr == nil {
		return errors.New("invalid critic output did not fail the workflow machine")
	}
	if err := assertRootTerminal(workspace, run.Session, "failed"); err != nil {
		return err
	}
	trace, err := os.ReadFile(run.Trace)
	if err != nil {
		return err
	}
	if !bytes.Contains(trace, []byte("final machine state: Failed")) {
		return errors.New("invalid critic output did not fail the critic child")
	}

	var manifest recordedManifest
	if err := readJSON(filepath.Join(workspace, "manifest.json"), &manifest); err != nil {
		return err
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Stage == "critique" || artifact.Stage == "final" {
			return fmt.Errorf("invalid critic output persisted %s artifact %s", artifact.Stage, artifact.ID)
		}
	}
	if manifest.Selected["critique"] != "" || manifest.Selected["final"] != "" {
		return errors.New("invalid critic output entered accepted lineage")
	}
	for _, path := range []string{"40-critique.json", "final.md"} {
		if _, err := os.Stat(filepath.Join(workspace, path)); !os.IsNotExist(err) {
			return fmt.Errorf("invalid critic output materialized %s", path)
		}
	}
	receipts, err := loadReceipts(workspace)
	if err != nil {
		return err
	}
	for _, receipt := range receipts {
		if receipt.Session != run.Session {
			continue
		}
		if receipt.Operation == "write-critique-attempt" || receipt.Operation == "materialize-final-chain" {
			return fmt.Errorf("invalid critic output reached boundary operation %s", receipt.Operation)
		}
	}
	fmt.Printf("interpreter scenario %s: workflow=Failed critique_persisted=false final_materialized=false\n", scenario.Name)
	return nil
}

func runInterpreterFailureEvidence(root string, runtime tracerRuntime, suite interpreterSuite) error {
	happy, ok := scenarioByName(suite, "happy")
	if !ok {
		return errors.New("happy interpreter scenario missing")
	}
	for _, failure := range []struct {
		session string
		fault   string
	}{
		{session: "source-error", fault: "capture-source:1"},
		{session: "rag-error", fault: "http:/api/v1/rag/query:1"},
		{session: "model-error", fault: "http:/api/chat:1"},
	} {
		workspace, err := os.MkdirTemp("", "prose-editor-"+failure.session+"-*")
		if err != nil {
			return err
		}
		_, runErr := runAgentSession(runtime, happy, workspace, failure.session, failure.fault)
		if runErr == nil {
			_ = os.RemoveAll(workspace)
			return fmt.Errorf("injected %s did not fail the workflow machine", failure.fault)
		}
		if err := assertRootTerminal(workspace, failure.session, "failed"); err != nil {
			_ = os.RemoveAll(workspace)
			return err
		}
		_ = os.RemoveAll(workspace)
	}

	retry, ok := scenarioByName(suite, "reject-then-accept")
	if !ok {
		return errors.New("retry interpreter scenario missing")
	}
	workspace, err := os.MkdirTemp("", "prose-editor-recovery-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	if _, err = runAgentSession(runtime, retry, workspace, "interrupted", "append-manifest-revision:2"); err == nil {
		return errors.New("injected durable-cut interruption did not fail the workflow machine")
	}
	if err := assertRootTerminal(workspace, "interrupted", "failed"); err != nil {
		return err
	}
	recovered, err := runAgentSession(runtime, retry, workspace, "recovered", "")
	if err != nil {
		return fmt.Errorf("restart after durable cut: %w", err)
	}
	if err := validateInterpreterWorkspace(workspace, retry, recovered); err != nil {
		return fmt.Errorf("restart after durable cut: %w", err)
	}
	receipts, err := loadReceipts(workspace)
	if err != nil {
		return err
	}
	if !sessionHasReplay(receipts, "recovered") {
		return errors.New("recovery restart did not reuse completed boundary mutations")
	}
	return nil
}

func runAgentSession(
	runtime tracerRuntime,
	scenario interpreterScenario,
	workspace, session, fault string,
) (interpreterRun, error) {
	env := tracerEnvironment(runtime, scenario, workspace, session, fault)
	server, serverOut, serverErr, err := startBoundaryServer(runtime, env)
	if err != nil {
		return interpreterRun{}, err
	}
	defer stopBoundaryServer(server)

	request := filepath.Join(workspace, ".tracer", "root-request-"+session+".json")
	if err := os.MkdirAll(filepath.Dir(request), 0o755); err != nil {
		return interpreterRun{}, err
	}
	requestData, _ := json.Marshal(map[string]string{"scenario": scenario.Name, "saga_id": scenario.SagaID})
	if err := os.WriteFile(request, requestData, 0o444); err != nil {
		return interpreterRun{}, err
	}
	trace := filepath.Join(workspace, ".evidence", "root-"+session+".otel.json")
	if err := os.MkdirAll(filepath.Dir(trace), 0o755); err != nil {
		return interpreterRun{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, runtime.agent,
		"--profile", runtime.orchestrator,
		"--core-root", runtime.coreRoot,
		"--directory", workspace,
		"--request", request,
		"--child-agent-binary", runtime.agent,
		"--otel-log-file", trace,
		"--otel-service-name", "prose-workflow-orchestrator",
	)
	command.Dir = runtime.workflowCWD
	command.Env = env
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if ctx.Err() != nil {
		runErr = fmt.Errorf("agent timed out: %w", ctx.Err())
	}
	_ = os.WriteFile(filepath.Join(workspace, ".evidence", "root-"+session+".stdout"), stdout.Bytes(), 0o644)
	_ = os.WriteFile(filepath.Join(workspace, ".evidence", "root-"+session+".stderr"), stderr.Bytes(), 0o644)
	stopBoundaryServer(server)
	result := interpreterRun{Session: session, Stdout: stdout.String(), Stderr: stderr.String(), Trace: trace}
	if runErr != nil {
		traceData, _ := os.ReadFile(trace)
		return result, fmt.Errorf(
			"agent-core workflow execution failed: %w\nroot stderr:\n%s\nboundary stdout:\n%s\nboundary stderr:\n%s\nroot trace:\n%s",
			runErr, stderr.String(), serverOut.String(), serverErr.String(), traceData,
		)
	}
	return result, nil
}

func tracerEnvironment(
	runtime tracerRuntime,
	scenario interpreterScenario,
	workspace, session, fault string,
) []string {
	//nolint:forbidigo // Reads PATH only to prepend the build directory for the child, which the go-style constitution allows as a PATH prepend on a child contract (GH-1481).
	path := runtime.buildDir + string(os.PathListSeparator) + os.Getenv("PATH")
	modelAddress := runtime.modelListener.Addr().String()
	ragAddress := runtime.ragListener.Addr().String()
	modelPort := strconv.Itoa(runtime.modelListener.Addr().(*net.TCPAddr).Port)
	ragPort := strconv.Itoa(runtime.ragListener.Addr().(*net.TCPAddr).Port)
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"PATH="+path,
		"PROSE_TRACER_WORKSPACE="+workspace,
		"PROSE_TRACER_FIXTURES="+runtime.fixtures,
		"PROSE_TRACER_SCENARIO="+scenario.Name,
		"PROSE_TRACER_SESSION="+session,
		"PROSE_TRACER_FAULT="+fault,
		"PROSE_EDITOR_MODEL=tracer-editor",
		"PROSE_CRITIC_MODEL=tracer-critic",
		"PROSE_EDITOR_EMBEDDING_MODEL=tracer-embedding",
		"PROSE_TRACER_MODEL_PORT="+modelPort,
		"PROSE_TRACER_RAG_PORT="+ragPort,
		"OLLAMA_URL=http://"+modelAddress,
		"STRUCTURE_RAG_URL=http://"+ragAddress,
	)
	return env
}

func startBoundaryServer(runtime tracerRuntime, env []string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, error) {
	modelFile, err := runtime.modelListener.File()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("duplicate model boundary listener: %w", err)
	}
	ragFile, err := runtime.ragListener.File()
	if err != nil {
		_ = modelFile.Close()
		return nil, nil, nil, fmt.Errorf("duplicate RAG boundary listener: %w", err)
	}
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		_ = modelFile.Close()
		_ = ragFile.Close()
		return nil, nil, nil, fmt.Errorf("create boundary readiness pipe: %w", err)
	}
	defer readyReader.Close()
	command := exec.Command(runtime.boundary, "serve", "3", "4", "5")
	command.Dir = runtime.workflowCWD
	command.Env = env
	command.ExtraFiles = []*os.File{modelFile, ragFile, readyWriter}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command.Stdout, command.Stderr = stdout, stderr
	startErr := command.Start()
	_ = modelFile.Close()
	_ = ragFile.Close()
	_ = readyWriter.Close()
	if startErr != nil {
		return nil, stdout, stderr, startErr
	}
	ready := make(chan error, 1)
	go func() {
		var marker [1]byte
		_, err := readyReader.Read(marker[:])
		ready <- err
	}()
	select {
	case err := <-ready:
		if err == nil {
			return command, stdout, stderr, nil
		}
		stopBoundaryServer(command)
		return nil, stdout, stderr, fmt.Errorf("boundary server readiness failed: %w: %s", err, stderr.String())
	case <-time.After(5 * time.Second):
		stopBoundaryServer(command)
		<-ready
		return nil, stdout, stderr, fmt.Errorf("boundary server did not become ready: %s", stderr.String())
	}
}

func stopBoundaryServer(command *exec.Cmd) {
	if command == nil || command.Process == nil || command.ProcessState != nil {
		return
	}
	_ = command.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = command.Process.Kill()
		<-done
	}
}

func validateInterpreterWorkspace(
	workspace string,
	scenario interpreterScenario,
	run interpreterRun,
) error {
	if !strings.Contains(run.Stderr, "terminal state: succeeded") {
		return fmt.Errorf("workflow interpreter terminal missing from stderr: %s", run.Stderr)
	}
	expectedTerminal := expectedWorkflowTerminal(scenario)
	if !strings.Contains(run.Stderr, "final machine state: "+expectedTerminal) {
		return fmt.Errorf("workflow final machine state %s missing from stderr: %s", expectedTerminal, run.Stderr)
	}
	trace, err := os.ReadFile(run.Trace)
	if err != nil {
		return err
	}
	for _, proof := range []string{
		"self_invoke.profile",
		"specialist-editor/profile.yaml",
		"voice-critic/profile.yaml",
		"self_invoke.stderr",
		"terminal state: succeeded",
		"final machine state: Succeeded",
	} {
		if !bytes.Contains(trace, []byte(proof)) {
			return fmt.Errorf("root trace lacks child interpreter proof %q", proof)
		}
	}
	var manifest recordedManifest
	if err := readJSON(filepath.Join(workspace, "manifest.json"), &manifest); err != nil {
		return err
	}
	for _, response := range scenario.CriticResponses {
		childTerminal := "Rejected"
		if strings.Contains(response, "/pass.yaml") {
			childTerminal = "Passed"
		}
		if !bytes.Contains(trace, []byte("final machine state: "+childTerminal)) {
			return fmt.Errorf("root trace lacks critic final machine state %s", childTerminal)
		}
	}
	if manifest.SchemaVersion != "prose-editor.interpreter-trace/v1" ||
		manifest.SagaID != scenario.SagaID || manifest.Terminal != expectedTerminal {
		return fmt.Errorf("manifest identity/terminal = %s/%s/%s", manifest.SchemaVersion, manifest.SagaID, manifest.Terminal)
	}
	if err := validateSelectedLineage(manifest, scenario); err != nil {
		return err
	}
	structures, critiques, superseded := 0, 0, 0
	for _, artifact := range manifest.Artifacts {
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(artifact.Path)))
		if err != nil {
			return err
		}
		if digestBytes(data) != artifact.SHA256 {
			return fmt.Errorf("artifact %s hash differs from interpreter receipt", artifact.ID)
		}
		switch artifact.Stage {
		case "structure":
			structures++
			if artifact.Status == "superseded" {
				superseded++
			}
		case "critique":
			critiques++
			if artifact.Producer != "voice-critic" || len(artifact.Parents) != 2 {
				return errors.New("critic artifact lacks independent exact lineage")
			}
		}
	}
	if structures != len(scenario.EditorResponses) || critiques != len(scenario.CriticResponses) {
		return fmt.Errorf("machine attempts structure/critic = %d/%d, want %d/%d",
			structures, critiques, len(scenario.EditorResponses), len(scenario.CriticResponses))
	}
	if len(scenario.EditorResponses) > 1 && superseded != 1 {
		return fmt.Errorf("superseded structure attempts = %d, want 1", superseded)
	}
	if scenario.ExpectedTerminal == "locally_finalized" {
		for _, path := range []string{"10-structure.md", "40-critique.json", "final.md"} {
			if _, err := os.Stat(filepath.Join(workspace, path)); err != nil {
				return err
			}
		}
	} else if _, err := os.Stat(filepath.Join(workspace, "final.md")); !os.IsNotExist(err) {
		return errors.New("retry exhaustion materialized a final workproduct")
	}
	receipts, err := loadReceipts(workspace)
	if err != nil {
		return err
	}
	return validateSessionReceipts(receipts, run.Session, scenario)
}

func validateSelectedLineage(manifest recordedManifest, scenario interpreterScenario) error {
	if manifest.Selected["original"] == "" {
		return errors.New("manifest has no selected original")
	}
	if scenario.ExpectedTerminal == "kept_original" {
		for stage, id := range manifest.Selected {
			if stage != "original" && id != "" {
				return fmt.Errorf("KeptOriginal selected rejected %s artifact %s", stage, id)
			}
		}
		return nil
	}
	for _, stage := range []string{"structure", "critique", "final"} {
		if manifest.Selected[stage] == "" {
			return fmt.Errorf("finalized manifest has no selected %s", stage)
		}
	}
	return nil
}

func expectedWorkflowTerminal(scenario interpreterScenario) string {
	return map[string]string{
		"locally_finalized": "LocallyFinalized",
		"kept_original":     "KeptOriginal",
	}[scenario.ExpectedTerminal]
}

func validateSessionReceipts(receipts []recordedReceipt, session string, scenario interpreterScenario) error {
	counts := map[string]int{}
	for _, receipt := range receipts {
		if receipt.Session != session {
			continue
		}
		if receipt.Status != "ok" && receipt.Status != "selected" {
			return fmt.Errorf("session %s boundary receipt %s has status %s", session, receipt.Operation, receipt.Status)
		}
		lower := strings.ToLower(receipt.Operation)
		for _, forbidden := range []string{"git", "pangram", "voice_editor", "style_editor", "helm", "kind", "kubectl"} {
			if strings.Contains(lower, forbidden) {
				return fmt.Errorf("forbidden boundary operation %q", receipt.Operation)
			}
		}
		counts[receipt.Operation]++
	}
	if counts["model:tracer-editor"] != len(scenario.EditorResponses) ||
		counts["model:tracer-critic"] != len(scenario.CriticResponses) {
		return fmt.Errorf("recorded child model calls editor/critic = %d/%d, want %d/%d",
			counts["model:tracer-editor"], counts["model:tracer-critic"],
			len(scenario.EditorResponses), len(scenario.CriticResponses))
	}
	if counts["write-structure-attempt"] != len(scenario.EditorResponses) ||
		counts["write-critique-attempt"] != len(scenario.CriticResponses) {
		return errors.New("recorded boundary attempts do not match child-machine executions")
	}
	return nil
}

func manifestSemantics(workspace string) (any, error) {
	data, err := os.ReadFile(filepath.Join(workspace, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func immutableArtifactDigest(workspace string) (map[string]string, error) {
	var manifest recordedManifest
	if err := readJSON(filepath.Join(workspace, "manifest.json"), &manifest); err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, artifact := range manifest.Artifacts {
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(artifact.Path)))
		if err != nil {
			return nil, err
		}
		result[artifact.Path] = digestBytes(data)
	}
	return result, nil
}

func loadReceipts(workspace string) ([]recordedReceipt, error) {
	data, err := os.ReadFile(filepath.Join(workspace, "boundary-receipts.jsonl"))
	if err != nil {
		return nil, err
	}
	var receipts []recordedReceipt
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		var receipt recordedReceipt
		if err := json.Unmarshal(line, &receipt); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func sessionHasReplay(receipts []recordedReceipt, session string) bool {
	for _, receipt := range receipts {
		if receipt.Session == session && receipt.Replay {
			return true
		}
	}
	return false
}

func scenarioByName(suite interpreterSuite, name string) (interpreterScenario, bool) {
	for _, scenario := range suite.Scenarios {
		if scenario.Name == name {
			return scenario, true
		}
	}
	return interpreterScenario{}, false
}

func assertRootTerminal(workspace, session, terminal string) error {
	data, err := os.ReadFile(filepath.Join(workspace, ".evidence", "root-"+session+".stderr"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "terminal state: "+terminal) {
		return fmt.Errorf("root session %s did not reach %s: %s", session, terminal, data)
	}
	return nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
