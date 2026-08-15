// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	chromaCorpusFixture        = "testdata/integration/chroma-corpus"
	chromaIngestProfile        = "agents/corpus-ingest/profile.yaml"
	corpusRestAsset            = "agents/corpus-ingest/corpus-rest.yaml"
	corpusIngestLibraryProfile = "agents/knowledge-manager/corpus-ingest/profile.yaml"

	chromaImage = "chromadb/chroma:1.5.3"

	chromaHeartbeatURL = "http://127.0.0.1:8000/api/v2/heartbeat"

	// chromaReuseProbeTimeout bounds the "is a Chroma already there?" question.
	// It is short because the answer is local and immediate, and every run with
	// no Chroma present pays it before starting its own.
	chromaReuseProbeTimeout = 2 * time.Second
	ollamaVersionURL        = "http://127.0.0.1:11434/api/version"
	ollamaTagsURL           = "http://127.0.0.1:11434/api/tags"
	ollamaProcessesURL      = "http://127.0.0.1:11434/api/ps"
)

// Chroma proves the corpus-ingest profile against a live Chroma server run from
// the chromadb/chroma Docker container and a live Ollama provider: ingest loads
// the corpus fixture and the collection count verifies documents were written.
// The seeded collection is what the rag-server and chatbot targets query. The
// target skips (does not fail) when Docker or Ollama with the configured chat and
// embedding models is unavailable, so the group stays usable without them.
func (Integration) Chroma() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(profilesRoot)
	if err := requireProfilePaths(profilesRoot, chromaIngestProfile, corpusRestAsset); err != nil {
		return err
	}
	chatModel := demoChromaIntegrationChatModel(profilesRoot)
	requiredModels, err := chromaRequiredModelsForChat(profilesRoot, chatModel)
	if err != nil {
		return fmt.Errorf("invalid shipped Chroma model config: %w", err)
	}
	if reason := chromaOllamaSkipReasonForModels(requiredModels); reason != "" {
		fmt.Printf("SKIP chroma: %s\n", reason)
		return nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("SKIP chroma: docker not found on PATH")
		return nil
	}
	return runChromaIntegration(profilesRoot, coreRoot, chatModel)
}

// Seed loads the application corpus into a running Chroma through the mesh wrapper
// around the canonical corpus-ingest profile, so a developer can populate the collection the rag-server
// and chatbot serve. It embeds through Ollama and writes to Chroma at the declared
// local ports. Skips cleanly when agent-core, Ollama with the configured models,
// or Chroma is unavailable.
func Seed() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(profilesRoot)
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP seed: agent-core checkout not found at %s (set core_root in demo.yaml)\n", coreRoot)
		return nil
	}
	if err := requireProfilePaths(profilesRoot, chromaIngestProfile, corpusRestAsset); err != nil {
		return err
	}
	requiredModels, err := chromaRequiredModels(profilesRoot)
	if err != nil {
		return fmt.Errorf("invalid shipped Chroma model config: %w", err)
	}
	if reason := chromaOllamaSkipReasonForModels(requiredModels); reason != "" {
		fmt.Printf("SKIP seed: %s\n", reason)
		return nil
	}
	if err := waitHTTPStatus(chromaHeartbeatURL, http.StatusOK, 2*time.Second); err != nil {
		fmt.Printf("SKIP seed: Chroma not reachable at %s: %v\n", chromaHeartbeatURL, err)
		return nil
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(binary) }()
	if err := runChromaIngest(binary, profilesRoot, coreRoot, ""); err != nil {
		return err
	}
	fmt.Println("seed: corpus ingested into Chroma")
	return nil
}

func chromaOllamaSkipReasonForModels(required []string) string {
	if err := waitHTTPStatus(ollamaVersionURL, http.StatusOK, 2*time.Second); err != nil {
		return fmt.Sprintf("Ollama not reachable at %s: %v", ollamaVersionURL, err)
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

// chromaRequiredModels returns the distinct Ollama model names the ingest profile
// uses: the embedding model from the REST asset and the invoke_llm chat model from
// the profile's declarations. Reading them from the config keeps it the single
// source of truth for the skip gate.
func chromaRequiredModels(profilesRoot string) ([]string, error) {
	return chromaRequiredModelsForChat(profilesRoot, "")
}

func chromaRequiredModelsForChat(
	profilesRoot, selectedChatModel string,
) ([]string, error) {
	set := map[string]bool{}
	embed, err := chromaEmbedModelFromConfig(profilesRoot)
	if err != nil {
		return nil, err
	}
	set[embed] = true
	chat := strings.TrimSpace(selectedChatModel)
	if chat == "" {
		chat, err = chromaChatModelFromConfig(profilesRoot, "corpus-ingest")
		if err != nil {
			return nil, err
		}
	}
	set[chat] = true
	models := make([]string, 0, len(set))
	for model := range set {
		models = append(models, model)
	}
	sort.Strings(models)
	return models, nil
}

// chromaEmbedModelFromConfig reads the embedding model from the ollama embed
// operation in the corpus-ingest corpus-rest.yaml.
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
	path := filepath.Join(profilesRoot, corpusRestAsset)
	if err := readIntegrationYAML(path, "chroma rest asset", &cfg); err != nil {
		return "", err
	}
	model := resolveModelReference(cfg.Rest.Clients["ollama"].Operations["embed"].Body.Model)
	if model == "" {
		return "", fmt.Errorf("no ollama embed model in %s", path)
	}
	return model, nil
}

// chromaChatModelFromConfig reads the invoke_llm chat model from a profile's
// declarations.yaml.
func chromaChatModelFromConfig(profilesRoot, profile string) (string, error) {
	var cfg struct {
		Tools []struct {
			Name   string `yaml:"name"`
			Config struct {
				Model string `yaml:"model"`
			} `yaml:"config"`
		} `yaml:"tools"`
	}
	path := filepath.Join(profilesRoot, "agents", profile, "declarations.yaml")
	if profile == "corpus-ingest" {
		catalogRoot, err := corpusIngestLibraryRoot(profilesRoot)
		if err != nil {
			return "", err
		}
		path = filepath.Join(
			catalogRoot,
			"agents", "knowledge-manager", "corpus-ingest", "declarations.yaml")
	}
	if err := readIntegrationYAML(path, "chroma declarations", &cfg); err != nil {
		return "", err
	}
	for _, tool := range cfg.Tools {
		if tool.Name == "invoke_llm" {
			model := resolveModelReference(tool.Config.Model)
			if model == "" {
				return "", fmt.Errorf("invoke_llm has no model in %s", path)
			}
			return model, nil
		}
	}
	return "", fmt.Errorf("no invoke_llm tool in %s", path)
}

var modelReference = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*):-([^}]*)\}$`)

// resolveModelReference mirrors the runtime's ${VAR:-default} selection for
// integration preflights that read unexpanded profile YAML.
func resolveModelReference(model string) string {
	match := modelReference.FindStringSubmatch(model)
	if match == nil {
		return model
	}
	//nolint:forbidigo // This preflight mirrors srd013 R5.6/R5.7 expansion before runtime loading.
	if value, ok := os.LookupEnv(match[1]); ok {
		return value
	}
	return match[2]
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

func runChromaIntegration(profilesRoot, coreRoot, chatModel string) error {
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	dataDir, err := os.MkdirTemp("", "chatbot-mesh-chroma-data-*")
	if err != nil {
		return fmt.Errorf("create chroma data dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dataDir) }()
	containerID, err := startRequiredChromaContainer(dataDir, ensureChromaServer)
	if err != nil {
		return err
	}
	defer stopChromaContainer(containerID)
	if err := runChromaIngest(binary, profilesRoot, coreRoot, chatModel); err != nil {
		return err
	}
	fmt.Println("integration:chroma PASS - ingest loaded the corpus and the collection count verified documents were written")
	return nil
}

func startRequiredChromaContainer(dataDir string, start func(string) (string, error)) (string, error) {
	containerID, err := start(dataDir)
	if err != nil {
		return "", fmt.Errorf("start required Chroma container: %w", err)
	}
	return containerID, nil
}

// ensureChromaServer reuses a Chroma already serving the port and otherwise
// starts one. It returns a container id only when this run created it, so the
// empty id is the created-by-me guard: stopChromaContainer ignores it, and a
// Chroma the operator started by hand survives the run. This mirrors the kind
// helper's reuse-and-do-not-delete rule (GH-589) and makes the target compose
// with a pre-existing Chroma that previously killed it on a port conflict
// (GH-708).
func ensureChromaServer(dataDir string) (string, error) {
	if waitHTTPStatus(chromaHeartbeatURL, http.StatusOK, chromaReuseProbeTimeout) == nil {
		fmt.Printf("chroma: reusing the server already answering %s; it will not be removed. "+
			"Its image and data are not this target's %s and %s, so a corpus collection left "+
			"there at a different embedding dimension will fail the query\n",
			chromaHeartbeatURL, chromaImage, dataDir)
		return "", nil
	}
	return startChromaContainer(dataDir)
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
		return "", chromaLaunchError(image, err, strings.TrimSpace(string(out)))
	}
	containerID := strings.TrimSpace(string(out))
	if err := waitHTTPStatus(chromaHeartbeatURL, http.StatusOK, 60*time.Second); err != nil {
		stopChromaContainer(containerID)
		return "", fmt.Errorf("chroma container served no heartbeat: %w", err)
	}
	return containerID, nil
}

// chromaLaunchError names the cause when the port is held by something that is
// not a Chroma. Reuse already handled a healthy one, so reaching here means the
// occupant never answered the heartbeat, and the bare docker "exit status 125"
// sends the reader to the daemon rather than to the process holding the port.
func chromaLaunchError(image string, err error, detail string) error {
	for _, marker := range []string{"port is already allocated", "address already in use"} {
		if strings.Contains(detail, marker) {
			return fmt.Errorf(
				"port 8000 is held by a process that does not answer the Chroma heartbeat at %s. "+
					"Stop it, or start a Chroma there and this target will reuse it, then re-run "+
					"(docker run %s: %v: %s)",
				chromaHeartbeatURL, image, err, detail)
		}
	}
	return fmt.Errorf("docker run %s: %v: %s", image, err, detail)
}

func stopChromaContainer(containerID string) {
	if containerID == "" {
		return
	}
	_ = exec.Command("docker", "rm", "-f", containerID).Run()
}

func runChromaIngest(binary, profilesRoot, coreRoot, chatModel string) error {
	corpusDir := filepath.Join(profilesRoot, chromaCorpusFixture, "corpus")
	ingestTimeout := demoChromaIngestTimeout(profilesRoot)
	runtimeRoot, cleanupRuntime, err := stageCorpusIngestRuntime(profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupRuntime()
	trace, cleanup, err := chromaTraceFile("ingest")
	if err != nil {
		return err
	}
	defer cleanup()
	profile := filepath.Join(runtimeRoot, chromaIngestProfile)
	if err := runChromaAgent(
		binary, runtimeRoot, coreRoot, profile, corpusDir, trace, ingestTimeout,
		chatModel); err != nil {
		model := strings.TrimSpace(chatModel)
		if model == "" {
			model = "profile default"
		}
		return fmt.Errorf("chroma ingest run failed with chat model %q: %w\n%s",
			model, err, chromaOllamaProcessDiagnostics())
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

func corpusIngestLibraryRoot(meshRoot string) (string, error) {
	if info, err := os.Stat(filepath.Join(
		meshRoot, filepath.FromSlash(corpusIngestLibraryProfile))); err == nil && !info.IsDir() {
		return meshRoot, nil
	}
	return resolveCatalogRoot("chatbot-mesh corpus ingest", meshRoot)
}

func stageCorpusIngestRuntime(meshRoot string) (string, func(), error) {
	stage, err := os.MkdirTemp("", "chatbot-mesh-corpus-ingest-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	if err := copyDirContents(
		filepath.Join(meshRoot, "agents", "corpus-ingest"),
		filepath.Join(stage, "agents", "corpus-ingest")); err != nil {
		cleanup()
		return "", nil, err
	}
	libraryRoot, err := corpusIngestLibraryRoot(meshRoot)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if err := copyDirContents(
		filepath.Join(libraryRoot, "agents", "knowledge-manager", "corpus-ingest"),
		filepath.Join(stage, "agents", "knowledge-manager", "corpus-ingest")); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage canonical corpus-ingest profile: %w", err)
	}
	return stage, cleanup, nil
}

func runChromaAgent(
	binary, profilesRoot, coreRoot, profile, directory, tracePath string,
	timeout time.Duration,
	chatModel string,
) error {
	args := []string{
		"--profile", profile,
		"--directory", directory,
		"--core-root", coreRoot,
		"--verbose-trace",
		"--otel-log-file", tracePath,
	}
	telemetryArgs, resourceEnv := hostIntegrationTelemetry("integration:chroma", "corpus-ingest", profilesRoot)
	return runChromaAgentWithTimeout(
		timeout,
		func(ctx context.Context) ([]byte, error) {
			cmd := exec.CommandContext(ctx, binary, append(args, telemetryArgs...)...)
			cmd.Dir = profilesRoot
			cmd.Env = chromaChildEnvironment(
				os.Environ(), resourceEnv, strings.TrimSpace(chatModel))
			// CombinedOutput calls Wait after CommandContext kills the child, so
			// timeout reporting cannot race process reaping or runtime cleanup.
			return cmd.CombinedOutput()
		},
	)
}

func chromaChildEnvironment(
	inherited []string,
	resourceEnv, chatModel string,
) []string {
	overrides := map[string]string{
		"OTEL_RESOURCE_ATTRIBUTES": resourceEnv,
	}
	if chatModel != "" {
		overrides["CORPUS_CHAT_MODEL"] = "CORPUS_CHAT_MODEL=" + chatModel
	}
	environment := make([]string, 0, len(inherited)+len(overrides))
	for _, entry := range inherited {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; replaced {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, resourceEnv)
	if chatModel != "" {
		environment = append(environment, "CORPUS_CHAT_MODEL="+chatModel)
	}
	return environment
}

func chromaOllamaProcessDiagnostics() string {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(ollamaProcessesURL)
	if err != nil {
		return fmt.Sprintf("Ollama loaded-model diagnostics unavailable: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Sprintf("Ollama loaded-model diagnostics returned %s",
			response.Status)
	}
	var result struct {
		Models []struct {
			Name          string `json:"name"`
			Model         string `json:"model"`
			ContextLength int    `json:"context_length"`
			SizeVRAM      int64  `json:"size_vram"`
		} `json:"models"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return fmt.Sprintf("Ollama loaded-model diagnostics could not decode: %v",
			err)
	}
	if len(result.Models) == 0 {
		return "Ollama loaded models: none"
	}
	details := make([]string, 0, len(result.Models))
	for _, model := range result.Models {
		name := model.Name
		if name == "" {
			name = model.Model
		}
		details = append(details, fmt.Sprintf(
			"%s(context=%d,vram=%d)", name, model.ContextLength, model.SizeVRAM))
	}
	sort.Strings(details)
	return "Ollama loaded models: " + strings.Join(details, ", ")
}

// chromaAgentRun is the context-aware command boundary for the canonical ingest.
// Tests substitute a deterministic runner to exercise deadline and reaping
// behavior without starting an agent or waiting out the production budget.
type chromaAgentRun func(context.Context) ([]byte, error)

func runChromaAgentWithTimeout(timeout time.Duration, run chromaAgentRun) error {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()
	out, err := run(ctx)
	elapsed := time.Since(started).Round(time.Millisecond)
	if contextErr := ctx.Err(); contextErr != nil {
		if detail := strings.TrimSpace(string(out)); detail != "" {
			return fmt.Errorf(
				"chroma ingest exceeded its %s whole-run timeout after %s (deadline %s; set chroma_ingest_timeout in %s to change it): %w\n%s",
				timeout, elapsed, deadline.UTC().Format(time.RFC3339Nano), demoConfigFile, contextErr, detail)
		}
		return fmt.Errorf(
			"chroma ingest exceeded its %s whole-run timeout after %s (deadline %s; set chroma_ingest_timeout in %s to change it): %w",
			timeout, elapsed, deadline.UTC().Format(time.RFC3339Nano), demoConfigFile, contextErr)
	}
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	// The exit code above already rules out a failed terminal, which exits 2
	// (agent-core srd018 R6.1/R6.2, GH-683). This asserts the terminal state
	// itself because a suspended run also exits 0 -- a deliberate pause with a
	// persisted checkpoint is not a failure (R6.4) -- and a tracer that ingested
	// nothing because its machine parked mid-run has not proven the flow.
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
	f, err := os.CreateTemp("", "chatbot-mesh-chroma-"+label+"-*.ndjson")
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

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
