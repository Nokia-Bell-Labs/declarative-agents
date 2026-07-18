// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	chromaChatModelEnv  = "AGENT_CORE_OLLAMA_MODEL"
	chromaEmbedModelEnv = "AGENT_CORE_OLLAMA_EMBED_MODEL"
	chromaChatModel     = "ornith:9b"
	chromaEmbedModel    = "all-minilm"

	chromaCorpusFixture = "testdata/integration/rel08-chroma-corpus"
	chromaIngestProfile = "agents/chroma/ingest/profile.yaml"
	chromaReaderProfile = "agents/chroma/reader/profile.yaml"

	chromaImageEnv = "AGENT_CORE_CHROMA_IMAGE"
	chromaImage    = "chromadb/chroma:1.5.3"

	chromaHeartbeatURL = "http://127.0.0.1:8000/api/v2/heartbeat"
	ollamaVersionURL   = "http://127.0.0.1:11434/api/version"
	ollamaTagsURL      = "http://127.0.0.1:11434/api/tags"
)

// Chroma proves the ingest and reader corpus profiles against a live Chroma
// server run from the chromadb/chroma Docker container and a live Ollama
// provider. Ingest loads a corpus fixture and verifies the collection count;
// the reader threads a provider-computed query vector into Chroma and grounds a
// model answer in the retrieved chunks. The target skips (does not fail) when
// Docker or Ollama with the configured chat and embedding models is
// unavailable, so the aggregate stays usable without the external services.
func (Integration) Chroma() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(profilesRoot), "agent-core"))
	if err := requireChromaProfiles(profilesRoot); err != nil {
		return err
	}
	if reason := chromaOllamaSkipReason(); reason != "" {
		fmt.Printf("SKIP chroma: %s\n", reason)
		return nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("SKIP chroma: docker not found on PATH")
		return nil
	}
	return runChromaIntegration(profilesRoot, coreRoot)
}

func requireChromaProfiles(profilesRoot string) error {
	return requireProfilePaths(profilesRoot, chromaIngestProfile, chromaReaderProfile, "agents/chroma/rest.yaml")
}

func configuredChromaChatModel() string {
	return envOrDefault(chromaChatModelEnv, chromaChatModel)
}

func configuredChromaEmbedModel() string {
	return envOrDefault(chromaEmbedModelEnv, chromaEmbedModel)
}

// chromaOllamaSkipReason returns a non-empty reason when Ollama is unreachable
// or the configured chat and embedding models are not both installed.
func chromaOllamaSkipReason() string {
	if err := waitHTTPStatus(ollamaVersionURL, http.StatusOK, 2*time.Second); err != nil {
		return fmt.Sprintf("Ollama not reachable at %s: %v", ollamaVersionURL, err)
	}
	names, err := fetchChromaOllamaModels()
	if err != nil {
		return fmt.Sprintf("Ollama /api/tags preflight failed: %v", err)
	}
	for _, model := range []string{configuredChromaChatModel(), configuredChromaEmbedModel()} {
		if !chromaModelInstalled(names, model) {
			return fmt.Sprintf("Ollama model %q not installed; available: %s", model, strings.Join(names, ", "))
		}
	}
	return ""
}

// chromaModelInstalled matches a configured model against the installed model
// names, tolerating the optional ":latest" tag Ollama omits in /api/tags.
func chromaModelInstalled(names []string, model string) bool {
	for _, name := range names {
		if name == model || name == model+":latest" || strings.TrimSuffix(name, ":latest") == model {
			return true
		}
	}
	return false
}

func fetchChromaOllamaModels() ([]string, error) {
	data, status, err := requestHTTP(http.MethodGet, ollamaTagsURL, "")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("/api/tags returned status %d", status)
	}
	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(result.Models))
	for _, model := range result.Models {
		if model.Name != "" {
			names = append(names, model.Name)
		}
	}
	return names, nil
}

func runChromaIntegration(profilesRoot, coreRoot string) error {
	binary, err := buildIntegrationAgent(coreRoot)
	if err != nil {
		return err
	}
	profileDir, cleanupProfiles, err := prepareChromaProfiles(profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupProfiles()
	dataDir, err := os.MkdirTemp("", "agent-profiles-chroma-data-*")
	if err != nil {
		return fmt.Errorf("create chroma data dir: %w", err)
	}
	defer os.RemoveAll(dataDir)
	containerID, err := startChromaContainer(dataDir)
	if err != nil {
		fmt.Printf("SKIP chroma: %s\n", err)
		return nil
	}
	defer stopChromaContainer(containerID)
	if err := runChromaIngest(binary, profilesRoot, profileDir, coreRoot); err != nil {
		return err
	}
	if err := runChromaReader(binary, profilesRoot, profileDir, coreRoot); err != nil {
		return err
	}
	fmt.Println("integration:chroma PASS - ingest loaded the corpus and the reader grounded an answer in retrieved chunks")
	return nil
}

// prepareChromaProfiles copies the agents/chroma profile tree to a temporary
// directory and rewrites the embedding and chat model names to the configured
// models, so AGENT_CORE_OLLAMA_MODEL and AGENT_CORE_OLLAMA_EMBED_MODEL select
// the models the agent actually calls (not only the skip gate). With the
// environment unset the shipped model names are preserved. The tree is copied
// whole so the profiles' relative references (machine, tools, ../rest.yaml)
// keep resolving.
func prepareChromaProfiles(profilesRoot string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "agent-profiles-chroma-profile-*")
	if err != nil {
		return "", nil, fmt.Errorf("create chroma profile dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	src := filepath.Join(profilesRoot, "agents", "chroma")
	if out, err := exec.Command("cp", "-a", src+"/.", tmp).CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy chroma profiles: %s: %w", strings.TrimSpace(string(out)), err)
	}
	replacements := map[string]string{
		"model: all-minilm":        "model: " + configuredChromaEmbedModel(),
		`model: "qwen3.6:35b-mlx"`: `model: "` + configuredChromaChatModel() + `"`,
	}
	for _, rel := range []string{
		"rest.yaml",
		filepath.Join("ingest", "declarations.yaml"),
		filepath.Join("reader", "declarations.yaml"),
	} {
		if err := rewriteChromaFile(filepath.Join(tmp, rel), replacements); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return tmp, cleanup, nil
}

func rewriteChromaFile(path string, replacements map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	content := replaceAll(string(data), replacements)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// startChromaContainer runs the chromadb/chroma image detached with the
// persistence directory bind-mounted at /data and the v2 API published on
// 127.0.0.1:8000, then waits for the heartbeat. A missing Docker daemon, an
// unpullable image, or a heartbeat that never arrives is returned as an error
// so the caller can skip rather than fail. The image is overridable through
// the AGENT_CORE_CHROMA_IMAGE environment variable.
func startChromaContainer(dataDir string) (string, error) {
	image := envOrDefault(chromaImageEnv, chromaImage)
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"-p", "127.0.0.1:8000:8000",
		"-v", dataDir+":/data",
		image,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run %s: %v: %s", image, err, strings.TrimSpace(string(out)))
	}
	containerID := strings.TrimSpace(string(out))
	if err := waitHTTPStatus(chromaHeartbeatURL, http.StatusOK, 60*time.Second); err != nil {
		stopChromaContainer(containerID)
		return "", fmt.Errorf("chroma container served no heartbeat: %w", err)
	}
	return containerID, nil
}

func stopChromaContainer(containerID string) {
	if containerID == "" {
		return
	}
	_ = exec.Command("docker", "rm", "-f", containerID).Run()
}

func runChromaIngest(binary, profilesRoot, profileDir, coreRoot string) error {
	corpusDir := filepath.Join(profilesRoot, chromaCorpusFixture, "corpus")
	trace, cleanup, err := chromaTraceFile("ingest")
	if err != nil {
		return err
	}
	defer cleanup()
	profile := filepath.Join(profileDir, "ingest", "profile.yaml")
	if err := runChromaAgent(binary, profilesRoot, coreRoot, profile, corpusDir, trace); err != nil {
		return fmt.Errorf("chroma ingest run failed: %w", err)
	}
	if err := assertChromaIngestTrace(trace); err != nil {
		return err
	}
	count, err := chromaCollectionCount()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("ingest added no documents to the corpus collection")
	}
	return nil
}

func runChromaReader(binary, profilesRoot, profileDir, coreRoot string) error {
	workspace, err := os.MkdirTemp("", "agent-profiles-chroma-reader-*")
	if err != nil {
		return fmt.Errorf("create reader workspace: %w", err)
	}
	defer os.RemoveAll(workspace)
	trace, cleanup, err := chromaTraceFile("reader")
	if err != nil {
		return err
	}
	defer cleanup()
	profile := filepath.Join(profileDir, "reader", "profile.yaml")
	if err := runChromaAgent(binary, profilesRoot, coreRoot, profile, workspace, trace); err != nil {
		return fmt.Errorf("chroma reader run failed: %w", err)
	}
	return assertChromaReaderTrace(trace)
}

func runChromaAgent(binary, profilesRoot, coreRoot, profile, directory, tracePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary,
		"--profile", profile,
		"--directory", directory,
		"--core-root", coreRoot,
		"--verbose-trace",
		"--otel-log-file", tracePath,
	)
	cmd.Dir = profilesRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	// The agent exits zero even when the machine reaches a Failed terminal, so
	// the exit code alone does not prove the run succeeded.
	if !strings.Contains(string(out), "status=succeeded") {
		return fmt.Errorf("agent did not reach a succeeded terminal state:\n%s", out)
	}
	return nil
}

// chromaCollectionCount resolves the corpus collection and reads its item count
// directly from Chroma, so the ingest assertion checks that documents were
// actually written rather than only that the flow ran.
func chromaCollectionCount() (int, error) {
	base := "http://127.0.0.1:8000/api/v2/tenants/default_tenant/databases/default_database/collections"
	data, status, err := requestHTTP(http.MethodPost, base, `{"name":"corpus","get_or_create":true}`)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return 0, fmt.Errorf("resolve corpus collection: status %d: %s", status, data)
	}
	var collection struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &collection); err != nil {
		return 0, fmt.Errorf("decode collection id: %w", err)
	}
	countData, countStatus, err := requestHTTP(http.MethodGet, base+"/"+collection.ID+"/count", "")
	if err != nil {
		return 0, err
	}
	if countStatus != http.StatusOK {
		return 0, fmt.Errorf("read collection count: status %d: %s", countStatus, countData)
	}
	var count int
	if err := json.Unmarshal(countData, &count); err != nil {
		return 0, fmt.Errorf("decode collection count: %w", err)
	}
	return count, nil
}

func chromaTraceFile(label string) (string, func(), error) {
	f, err := os.CreateTemp("", "agent-profiles-chroma-"+label+"-*.ndjson")
	if err != nil {
		return "", nil, fmt.Errorf("create %s trace file: %w", label, err)
	}
	path := f.Name()
	_ = f.Close()
	return path, func() { _ = os.Remove(path) }, nil
}

// assertChromaIngestTrace proves the ingest preconditions and the terminal
// count verification ran: the Chroma and Ollama readiness words and the
// chroma_count word each recorded a dispatch span.
func assertChromaIngestTrace(tracePath string) error {
	spans, err := readChromaSpans(tracePath)
	if err != nil {
		return err
	}
	present := chromaCommandSet(spans)
	for _, want := range []string{"chroma_ready", "ollama_ready", "chroma_count"} {
		if !present[want] {
			return fmt.Errorf("ingest trace missing %q dispatch; saw %v", want, sortedKeys(present))
		}
	}
	return nil
}

// assertChromaReaderTrace proves the reader threaded a provider-computed query
// vector into Chroma and grounded a model answer: the embed_query, chroma_query,
// and invoke_llm words dispatched in that order and a final answer was recorded.
func assertChromaReaderTrace(tracePath string) error {
	spans, err := readChromaSpans(tracePath)
	if err != nil {
		return err
	}
	want := []string{"embed_query", "chroma_query", "invoke_llm"}
	if err := assertChromaCommandOrder(spans, want); err != nil {
		return err
	}
	if !chromaLLMAnswered(spans) {
		return fmt.Errorf("reader trace shows no grounded answer (invoke_llm produced no output tokens)")
	}
	return nil
}

// chromaLLMAnswered reports whether the reader's invoke_llm dispatch produced a
// model answer, evidenced by a positive output-token count on its span.
func chromaLLMAnswered(spans []chromaSpan) bool {
	for _, span := range spans {
		if span.commandName() != "invoke_llm" {
			continue
		}
		if tokens, ok := span.numericAttr("gen_ai.usage.output_tokens"); ok && tokens > 0 {
			return true
		}
	}
	return false
}

// assertChromaCommandOrder checks that the named tool words dispatched in the
// given order. Spans are ordered by start time because the batch exporter
// flushes them in completion order, not dispatch order.
func assertChromaCommandOrder(spans []chromaSpan, want []string) error {
	wanted := make(map[string]bool, len(want))
	for _, name := range want {
		wanted[name] = true
	}
	var got []string
	for _, span := range spans {
		if name := span.commandName(); wanted[name] {
			got = append(got, name)
		}
	}
	if !isChromaSubsequence(want, got) {
		return fmt.Errorf("reader trace command order = %v, want %v as a subsequence", got, want)
	}
	return nil
}

// isChromaSubsequence reports whether want appears in got in order, allowing
// unrelated dispatches (or repeats) between the wanted words.
func isChromaSubsequence(want, got []string) bool {
	i := 0
	for _, name := range got {
		if i < len(want) && name == want[i] {
			i++
		}
	}
	return i == len(want)
}

func chromaCommandSet(spans []chromaSpan) map[string]bool {
	present := make(map[string]bool)
	for _, span := range spans {
		if name := span.commandName(); name != "" {
			present[name] = true
		}
	}
	return present
}

func readChromaSpans(tracePath string) ([]chromaSpan, error) {
	data, err := os.ReadFile(tracePath)
	if err != nil {
		return nil, fmt.Errorf("read trace %s: %w", tracePath, err)
	}
	var spans []chromaSpan
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var span chromaSpan
		if err := json.Unmarshal([]byte(line), &span); err != nil {
			continue
		}
		spans = append(spans, span)
	}
	sort.SliceStable(spans, func(i, j int) bool {
		return spans[i].start().Before(spans[j].start())
	})
	return spans, nil
}

type chromaSpan struct {
	Name       string          `json:"Name"`
	StartTime  string          `json:"StartTime"`
	Attributes []chromaTraceKV `json:"Attributes"`
}

type chromaTraceKV struct {
	Key   string `json:"Key"`
	Value struct {
		Value interface{} `json:"Value"`
	} `json:"Value"`
}

func (s chromaSpan) start() time.Time {
	t, err := time.Parse(time.RFC3339Nano, s.StartTime)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (s chromaSpan) commandName() string {
	name, _ := s.stringAttr("command.name")
	return name
}

func (s chromaSpan) stringAttr(key string) (string, bool) {
	for _, attr := range s.Attributes {
		if attr.Key == key {
			if value, ok := attr.Value.Value.(string); ok {
				return value, true
			}
		}
	}
	return "", false
}

func (s chromaSpan) numericAttr(key string) (float64, bool) {
	for _, attr := range s.Attributes {
		if attr.Key == key {
			if value, ok := attr.Value.Value.(float64); ok {
				return value, true
			}
		}
	}
	return 0, false
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
