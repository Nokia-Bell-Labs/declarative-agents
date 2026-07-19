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
	chromaCorpusFixture = "testdata/integration/rel08-chroma-corpus"
	chromaIngestProfile = "agents/chroma/ingest/profile.yaml"
	chromaReaderProfile = "agents/chroma/reader/profile.yaml"

	chromaImage = "chromadb/chroma:1.5.3"

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
	if reason := chromaOllamaSkipReason(profilesRoot); reason != "" {
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

// chromaOllamaSkipReason returns a non-empty reason when Ollama is unreachable,
// the chroma model config cannot be read, or a model the config uses is not
// installed. The required models come from the shipped config, so the gate never
// duplicates the model names.
func chromaOllamaSkipReason(profilesRoot string) string {
	if err := waitHTTPStatus(ollamaVersionURL, http.StatusOK, 2*time.Second); err != nil {
		return fmt.Sprintf("Ollama not reachable at %s: %v", ollamaVersionURL, err)
	}
	required, err := chromaRequiredModels(profilesRoot)
	if err != nil {
		return fmt.Sprintf("read chroma model config: %v", err)
	}
	names, err := fetchChromaOllamaModels()
	if err != nil {
		return fmt.Sprintf("Ollama /api/tags preflight failed: %v", err)
	}
	for _, model := range required {
		if !chromaModelInstalled(names, model) {
			return fmt.Sprintf("Ollama model %q not installed; available: %s", model, strings.Join(names, ", "))
		}
	}
	return ""
}

// chromaRequiredModels returns the distinct Ollama model names the chroma
// profiles use: the embedding model from the REST asset and the invoke_llm chat
// model from each profile's declarations. Reading them from the config keeps it
// the single source of truth for the skip gate.
func chromaRequiredModels(profilesRoot string) ([]string, error) {
	set := map[string]bool{}
	embed, err := chromaEmbedModelFromConfig(profilesRoot)
	if err != nil {
		return nil, err
	}
	set[embed] = true
	for _, profile := range []string{"ingest", "reader"} {
		chat, err := chromaChatModelFromConfig(profilesRoot, profile)
		if err != nil {
			return nil, err
		}
		set[chat] = true
	}
	models := make([]string, 0, len(set))
	for model := range set {
		models = append(models, model)
	}
	sort.Strings(models)
	return models, nil
}

// chromaEmbedModelFromConfig reads the embedding model from the ollama embed
// operation in agents/chroma/rest.yaml.
func chromaEmbedModelFromConfig(profilesRoot string) (string, error) {
	var cfg struct {
		Rest struct {
			Clients map[string]struct {
				Operations map[string]struct {
					Body struct {
						Model string `yaml:"model"`
					} `yaml:"body"`
				} `yaml:"operations"`
			} `yaml:"clients"`
		} `yaml:"rest"`
	}
	path := filepath.Join(profilesRoot, "agents", "chroma", "rest.yaml")
	if err := readIntegrationYAML(path, "chroma rest asset", &cfg); err != nil {
		return "", err
	}
	model := cfg.Rest.Clients["ollama"].Operations["embed"].Body.Model
	if model == "" {
		return "", fmt.Errorf("no ollama embed model in %s", path)
	}
	return model, nil
}

// chromaChatModelFromConfig reads the invoke_llm chat model from a chroma
// profile's declarations.yaml.
func chromaChatModelFromConfig(profilesRoot, profile string) (string, error) {
	var cfg struct {
		Tools []struct {
			Name   string `yaml:"name"`
			Config struct {
				Model string `yaml:"model"`
			} `yaml:"config"`
		} `yaml:"tools"`
	}
	path := filepath.Join(profilesRoot, "agents", "chroma", profile, "declarations.yaml")
	if err := readIntegrationYAML(path, "chroma declarations", &cfg); err != nil {
		return "", err
	}
	for _, tool := range cfg.Tools {
		if tool.Name == "invoke_llm" {
			if tool.Config.Model == "" {
				return "", fmt.Errorf("invoke_llm has no model in %s", path)
			}
			return tool.Config.Model, nil
		}
	}
	return "", fmt.Errorf("no invoke_llm tool in %s", path)
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
	if err := runChromaIngest(binary, profilesRoot, coreRoot); err != nil {
		return err
	}
	if err := runChromaReader(binary, profilesRoot, coreRoot); err != nil {
		return err
	}
	fmt.Println("integration:chroma PASS - ingest loaded the corpus and the reader grounded an answer in retrieved chunks")
	return nil
}

// startChromaContainer runs the chromadb/chroma image detached with the
// persistence directory bind-mounted at /data and the v2 API published on
// 127.0.0.1:8000, then waits for the heartbeat. A missing Docker daemon, an
// unpullable image, or a heartbeat that never arrives is returned as an error
// so the caller can skip rather than fail.
func startChromaContainer(dataDir string) (string, error) {
	image := chromaImage
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

func runChromaIngest(binary, profilesRoot, coreRoot string) error {
	corpusDir := filepath.Join(profilesRoot, chromaCorpusFixture, "corpus")
	trace, cleanup, err := chromaTraceFile("ingest")
	if err != nil {
		return err
	}
	defer cleanup()
	profile := filepath.Join(profilesRoot, chromaIngestProfile)
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

func runChromaReader(binary, profilesRoot, coreRoot string) error {
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
	profile := filepath.Join(profilesRoot, chromaReaderProfile)
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
	Name        string          `json:"Name"`
	StartTime   string          `json:"StartTime"`
	Attributes  []chromaTraceKV `json:"Attributes"`
	SpanContext chromaSpanRef   `json:"SpanContext"`
	Parent      chromaSpanRef   `json:"Parent"`
}

// chromaSpanRef is the id pair the OTel file exporter writes for a span and its
// parent, so a connected cross-agent trace can be asserted across span logs.
type chromaSpanRef struct {
	TraceID string `json:"TraceID"`
	SpanID  string `json:"SpanID"`
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
