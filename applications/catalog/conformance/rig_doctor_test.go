// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// rigDoctorProfile is the shipped family entry point every case here boots.
const rigDoctorProfile = "agents/rig-doctor/profile.yaml"

// fixtureTraceID is the trace the failing-rollout fixture's captured spool
// tail carries; both of its spans report the rollout that never came up.
const fixtureTraceID = "7b1c0e2a4d6f8a9b0c1d2e3f40516273"

// diagnosis is the document srd023 R3 requires the model to write.
type diagnosis struct {
	ProbableCause string `yaml:"probable_cause"`
	Confidence    string `yaml:"confidence"`
	Findings      []struct {
		Claim    string `yaml:"claim"`
		Evidence []struct {
			Path string `yaml:"path"`
			Line int    `yaml:"line"`
		} `yaml:"evidence"`
	} `yaml:"findings"`
	NextCommand string   `yaml:"next_command"`
	Unresolved  []string `yaml:"unresolved"`
}

// stageEvidence copies the checked-in fixture into a writable directory, since
// the run writes its diagnosis beside the evidence.
func stageEvidence(t *testing.T, fixture string) string {
	t.Helper()
	source := filepath.Join("..", "testdata", "conformance", "rig-doctor", fixture)
	target := t.TempDir()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	for _, entry := range entries {
		copyEvidenceEntry(t, filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name()))
	}
	return target
}

func copyEvidenceEntry(t *testing.T, source, target string) {
	t.Helper()
	info, err := os.Stat(source)
	if err != nil {
		t.Fatalf("stat %s: %v", source, err)
	}
	if !info.IsDir() {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", source, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", target, err)
		}
		return
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create %s: %v", target, err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}
	for _, entry := range entries {
		copyEvidenceEntry(t, filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name()))
	}
}

// toolCall renders one model turn in the envelope the qwen response profile
// parses ([tool_call]{json}[/tool_call]).
func toolCall(tool string, params map[string]any) string {
	encoded, err := json.Marshal(map[string]any{"tool": tool, "parameters": params})
	if err != nil {
		panic(err)
	}
	return "[tool_call]" + string(encoded) + "[/tool_call]"
}

// scriptedProvider answers /api/chat with one turn per call, so the loop is
// driven without a live model. It is the first scripted multi-turn loop in the
// catalog: the executor's own conformance is live-gated.
func scriptedProvider(t *testing.T, turns []string) (*httptest.Server, *atomic.Int32) {
	server, calls, _ := scriptedProviderRecording(t, turns)
	return server, calls
}

// scriptedProviderRecording also returns the prompts the loop sent, so a test
// can prove a tool result was fed back to the model rather than only that the
// tool ran.
func scriptedProviderRecording(
	t *testing.T, turns []string,
) (*httptest.Server, *atomic.Int32, *promptLog) {
	return scriptedProviderTurns(t, turns, false)
}

// scriptedProviderLooping repeats its last turn forever, which is how a test
// drives a model that never takes the repair it is offered.
func scriptedProviderLooping(
	t *testing.T, turns []string,
) (*httptest.Server, *atomic.Int32, *promptLog) {
	return scriptedProviderTurns(t, turns, true)
}

func scriptedProviderTurns(
	t *testing.T, turns []string, repeatLast bool,
) (*httptest.Server, *atomic.Int32, *promptLog) {
	t.Helper()
	var calls atomic.Int32
	prompts := &promptLog{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.6:35b-mlx"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/chat":
			body, _ := io.ReadAll(r.Body)
			prompts.add(string(body))
			turn := int(calls.Add(1)) - 1
			if turn >= len(turns) {
				if !repeatLast {
					t.Errorf("model called %d times, script has %d turns", turn+1, len(turns))
				}
				turn = len(turns) - 1
			}
			response, _ := json.Marshal(map[string]any{
				"message":           map[string]string{"role": "assistant", "content": turns[turn]},
				"eval_count":        8,
				"prompt_eval_count": 16,
			})
			_, _ = w.Write(response)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &calls, prompts
}

// promptLog records every prompt the loop sent the model.
type promptLog struct {
	mu      sync.Mutex
	entries []string
}

func (p *promptLog) add(body string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = append(p.entries, body)
}

func (p *promptLog) joined() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.entries, "\n")
}

// TestRigDoctorDiagnosed drives the shipped loop over the failing-rollout
// fixture with a scripted provider and checks the written diagnosis against
// the srd023 R3 shape, including that every cite resolves (AC2).
func TestRigDoctorDiagnosed(t *testing.T) {
	t.Parallel()
	evidence := stageEvidence(t, "failing-rollout")
	written := strings.Join([]string{
		"probable_cause: The smoke-chatbot Deployment never became available because its image tag chatbot-mesh:dev-8f21a3 does not exist in the cluster.",
		"confidence: high",
		"findings:",
		"  - claim: smoke-chatbot reports zero available replicas while smoke-collector is available.",
		"    evidence:",
		"      - path: namespace-da-helm-smoke-rollout.txt",
		"        line: 2",
		"  - claim: The pod failed to pull chatbot-mesh:dev-8f21a3 and settled into ImagePullBackOff.",
		"    evidence:",
		"      - path: namespace-da-helm-smoke-events.txt",
		"        line: 4",
		"      - path: namespace-da-helm-smoke-events.txt",
		"        line: 7",
		"next_command: kubectl describe pod smoke-chatbot-6d4f8b9c7-nk2vq -n da-helm-smoke",
		"unresolved:",
		"  - Whether the image was built and loaded into the cluster before the release was installed.",
		"",
	}, "\n")
	provider, calls := scriptedProvider(t, []string{
		toolCall("read", map[string]any{"path": "manifest.yaml"}),
		toolCall("read", map[string]any{"path": "namespace-da-helm-smoke-rollout.txt"}),
		toolCall("read", map[string]any{"path": "namespace-da-helm-smoke-events.txt"}),
		toolCall("query_list_traces", map[string]any{"page_size": 5}),
		toolCall("query_get_trace", map[string]any{"trace_id": fixtureTraceID}),
		toolCall("write", map[string]any{"path": "diagnosis.yaml", "content": written}),
		toolCall("done", map[string]any{"summary": "diagnosis written"}),
	})

	result := Run(t, RunConfig{
		Profile:   rigDoctorProfile,
		Directory: evidence,
		Env:       []string{"OLLAMA_URL=" + provider.URL},
	})
	result.RequireExit(t, 0)
	result.RequireTerminalState(t, "Diagnosed")
	if got := calls.Load(); got != 7 {
		t.Errorf("model turns = %d, want 7", got)
	}

	data, err := os.ReadFile(filepath.Join(evidence, "diagnosis.yaml"))
	if err != nil {
		t.Fatalf("read diagnosis: %v", err)
	}
	var written2 diagnosis
	if err := yaml.Unmarshal(data, &written2); err != nil {
		t.Fatalf("decode diagnosis: %v", err)
	}
	if written2.ProbableCause == "" || written2.NextCommand == "" || len(written2.Unresolved) == 0 {
		t.Errorf("diagnosis is missing required fields: %+v", written2)
	}
	switch written2.Confidence {
	case "high", "medium", "low":
	default:
		t.Errorf("confidence = %q, want high, medium, or low", written2.Confidence)
	}
	if !strings.Contains(written2.ProbableCause, "smoke-chatbot") {
		t.Errorf("probable cause does not name the failing Deployment: %q", written2.ProbableCause)
	}
	if len(written2.Findings) == 0 {
		t.Fatal("diagnosis carries no findings")
	}
	for _, finding := range written2.Findings {
		if finding.Claim == "" || len(finding.Evidence) == 0 {
			t.Errorf("finding without a claim or a cite: %+v", finding)
			continue
		}
		for _, cite := range finding.Evidence {
			requireCiteResolves(t, evidence, cite.Path, cite.Line)
		}
	}
}

// requireCiteResolves fails when a cited file is missing or the cited line is
// past its end: an unresolvable cite is worse than no claim (srd023 R3.2).
func requireCiteResolves(t *testing.T, evidence, path string, line int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(evidence, path))
	if err != nil {
		t.Errorf("cited file does not resolve: %v", err)
		return
	}
	if lines := strings.Count(string(data), "\n"); line < 1 || line > lines+1 {
		t.Errorf("cite %s:%d is outside the file's %d lines", path, line, lines)
	}
}

// TestRigDoctorNoEvidence proves the gate: an empty directory terminates
// before any model call, so a diagnosis of nothing is never written (AC3).
func TestRigDoctorNoEvidence(t *testing.T) {
	t.Parallel()
	evidence := t.TempDir()
	provider, calls := scriptedProvider(t, []string{
		toolCall("done", map[string]any{"summary": "should never be reached"}),
	})

	result := Run(t, RunConfig{
		Profile:   rigDoctorProfile,
		Directory: evidence,
		Env:       []string{"OLLAMA_URL=" + provider.URL},
	})
	result.RequireTerminalState(t, "NoEvidence")
	if got := calls.Load(); got != 0 {
		t.Errorf("model was called %d times for an empty evidence directory, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(evidence, "diagnosis.yaml")); !os.IsNotExist(err) {
		t.Errorf("NoEvidence wrote a diagnosis: %v", err)
	}
}

// TestRigDoctorReadOnlyManifest checks the declared vocabulary: the loop can
// read the evidence and write its diagnosis, and reaches no cluster (AC1).
func TestRigDoctorReadOnlyManifest(t *testing.T) {
	t.Parallel()
	var selection struct {
		Tools []string `yaml:"tools"`
	}
	unmarshalShipped(t, filepath.Join("agents", "rig-doctor", "tools.yaml"), &selection)
	want := map[string]bool{
		"list_files": true, "evidence_present": true, "read": true, "find": true,
		"query_list_traces": true, "query_get_trace": true,
		"write": true, "invoke_llm": true, "parse_response": true,
		"report_parse_error": true, "done": true,
		"seed_diagnosis_path": true, "read_diagnosis": true,
		"diagnosis_states_a_cause": true, "diagnosis_cites_findings": true,
	}
	for _, name := range selection.Tools {
		if !want[name] {
			t.Errorf("rig-doctor selects unexpected word %q", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("rig-doctor is missing words: %v", want)
	}

	closure := filepath.Join(ProfilesRoot(), "agents", "rig-doctor")
	err := filepath.WalkDir(closure, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			declaration := strings.TrimSpace(line)
			if strings.HasPrefix(declaration, "#") {
				continue // prose, including this family's own read-only charter
			}
			for _, forbidden := range []string{"type: exec", "binary:"} {
				if strings.Contains(declaration, forbidden) {
					return fmt.Errorf("%s declares %q; the rig-doctor runs no child process of its own",
						filepath.Base(path), forbidden)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

// TestRigDoctorQueriesTheCapturedSpool proves the spool words are dispatchable
// by the model rather than only by a machine action. They are external, which
// is what lets $tool resolve them at run time and what lets exhaustiveness
// union TracesListed and TraceRetrieved into the loop state (GH-2291).
func TestRigDoctorQueriesTheCapturedSpool(t *testing.T) {
	t.Parallel()
	evidence := stageEvidence(t, "failing-rollout")
	provider, calls, prompts := scriptedProviderRecording(t, []string{
		toolCall("query_list_traces", map[string]any{}),
		toolCall("query_get_trace", map[string]any{"trace_id": fixtureTraceID}),
		toolCall("write", map[string]any{"path": "diagnosis.yaml", "content": strings.Join([]string{
			"probable_cause: The smoke-chatbot rollout never became available.",
			"confidence: medium",
			"findings:",
			"  - claim: The captured rollout.wait span reports the deployment never became available.",
			"    evidence:",
			"      - path: traces/collector.ndjson",
			"        line: 2",
			"next_command: kubectl get deploy -n da-helm-smoke",
			"unresolved:",
			"  - Why the image the release asked for is absent.",
			"",
		}, "\n")}),
		toolCall("done", map[string]any{"summary": "diagnosis written"}),
	})

	result := Run(t, RunConfig{
		Profile:   rigDoctorProfile,
		Directory: evidence,
		Env:       []string{"OLLAMA_URL=" + provider.URL},
	})
	result.RequireExit(t, 0)
	result.RequireTerminalState(t, "Diagnosed")
	if got := calls.Load(); got != 4 {
		t.Errorf("model turns = %d, want 4", got)
	}
	result.RequireToolSpans(t, "query_list_traces", "query_get_trace")

	// The spool reads must come back through the conversation: a query that
	// silently skipped every line would still produce spans and a terminal.
	sent := prompts.joined()
	for _, want := range []string{fixtureTraceID, "helm.install", "rollout.wait"} {
		if !strings.Contains(sent, want) {
			t.Errorf("the model was never shown %q from the captured spool", want)
		}
	}
}

// validDiagnosis is a diagnosis that passes the read-back gate: decodable
// YAML naming a cause and carrying one cited finding.
const validDiagnosis = `probable_cause: The smoke-chatbot image tag does not exist.
confidence: high
findings:
  - claim: smoke-chatbot reports zero available replicas.
    evidence:
      - path: namespace-da-helm-smoke-rollout.txt
        line: 2
next_command: kubectl describe deploy smoke-chatbot -n da-helm-smoke
unresolved:
  - Whether the image was ever built.
`

// malformedDiagnosis reproduces what a real model wrote during GH-2290: the
// evidence entry's line key sits one column short of its sibling path, so the
// document does not decode as YAML at all.
const malformedDiagnosis = `probable_cause: The smoke-chatbot image tag does not exist.
confidence: high
findings:
   - claim: smoke-chatbot reports zero available replicas.
     evidence:
        - path: namespace-da-helm-smoke-rollout.txt
         line: 2
next_command: kubectl describe deploy smoke-chatbot -n da-helm-smoke
unresolved:
  - Whether the image was ever built.
`

// TestRigDoctorRepairsAnUndecodableDiagnosis is the GH-2294 regression: the
// model writes YAML that does not parse, the gate reads it back and returns
// it, and the repaired write reaches Diagnosed (srd023 R4.4).
func TestRigDoctorRepairsAnUndecodableDiagnosis(t *testing.T) {
	t.Parallel()
	evidence := stageEvidence(t, "failing-rollout")
	provider, calls, prompts := scriptedProviderRecording(t, []string{
		toolCall("write", map[string]any{"path": "diagnosis.yaml", "content": malformedDiagnosis}),
		toolCall("done", map[string]any{"summary": "diagnosis written"}),
		toolCall("write", map[string]any{"path": "diagnosis.yaml", "content": validDiagnosis}),
		toolCall("done", map[string]any{"summary": "diagnosis repaired"}),
	})

	result := Run(t, RunConfig{
		Profile:   rigDoctorProfile,
		Directory: evidence,
		Env:       []string{"OLLAMA_URL=" + provider.URL},
	})
	result.RequireExit(t, 0)
	result.RequireTerminalState(t, "Diagnosed")
	if got := calls.Load(); got != 4 {
		t.Errorf("model turns = %d, want 4: write, done, repair, done", got)
	}
	if sent := prompts.joined(); !strings.Contains(sent, "parse") && !strings.Contains(sent, "yaml") {
		t.Errorf("the model was never told why its diagnosis was rejected")
	}
	data, err := os.ReadFile(filepath.Join(evidence, "diagnosis.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var written diagnosis
	if err := yaml.Unmarshal(data, &written); err != nil {
		t.Fatalf("the accepted diagnosis does not decode: %v", err)
	}
}

// TestRigDoctorRepairsAMissingDiagnosis covers done called with nothing
// written: the gate returns it rather than reporting a run that produced no
// diagnosis as a success.
func TestRigDoctorRepairsAMissingDiagnosis(t *testing.T) {
	t.Parallel()
	evidence := stageEvidence(t, "failing-rollout")
	provider, calls, _ := scriptedProviderRecording(t, []string{
		toolCall("done", map[string]any{"summary": "nothing written"}),
		toolCall("write", map[string]any{"path": "diagnosis.yaml", "content": validDiagnosis}),
		toolCall("done", map[string]any{"summary": "diagnosis written"}),
	})

	result := Run(t, RunConfig{
		Profile:   rigDoctorProfile,
		Directory: evidence,
		Env:       []string{"OLLAMA_URL=" + provider.URL},
	})
	result.RequireTerminalState(t, "Diagnosed")
	if got := calls.Load(); got != 3 {
		t.Errorf("model turns = %d, want 3: done, write, done", got)
	}
}

// TestRigDoctorRefusesADiagnosisWithoutFindings proves the gate checks the
// decoded shape, not only that the bytes parse.
func TestRigDoctorRefusesADiagnosisWithoutFindings(t *testing.T) {
	t.Parallel()
	evidence := stageEvidence(t, "failing-rollout")
	bare := "probable_cause: Something failed.\nconfidence: low\nfindings: []\n" +
		"next_command: kubectl get pods -n da-helm-smoke\nunresolved: []\n"
	provider, _, _ := scriptedProviderLooping(t, []string{
		toolCall("write", map[string]any{"path": "diagnosis.yaml", "content": bare}),
		toolCall("done", map[string]any{"summary": "diagnosis written"}),
	})

	result := Run(t, RunConfig{
		Profile:   rigDoctorProfile,
		Directory: evidence,
		Env:       []string{"OLLAMA_URL=" + provider.URL},
		Timeout:   3 * time.Minute,
	})
	result.RequireTerminalState(t, "Failed")
}
