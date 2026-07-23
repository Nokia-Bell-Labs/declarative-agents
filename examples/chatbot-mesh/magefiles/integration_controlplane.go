// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	cpCoordinatorControl   = "http://127.0.0.1:18101/api/lifecycle/health"
	cpCoordinatorExit      = "http://127.0.0.1:18101/api/lifecycle/exit"
	cpCoordinatorProvision = "http://127.0.0.1:18100/api/v1/provision"
	cpCoordinatorState     = "http://127.0.0.1:18100/provisioning/api/state"
	cpCreatorControl       = "http://127.0.0.1:18111/api/lifecycle/health"
	cpCreatorExit          = "http://127.0.0.1:18111/api/lifecycle/exit"
	cpDeploymentAPIAddr    = "127.0.0.1:18090"
)

// ControlPlane proves the mesh control-plane flow end to end without a cluster: a
// provisioning intent flows chatbot -> coordinator -> creator -> deployment API. The
// coordinator and creator run as the real declarative agents; a fake deployment API
// stands in for the executor (srd006) and records what the creator sends. The test drives
// the intent the way the chatbot's provisioning panel does -- a POST through the
// coordinator's declared intent endpoint -- then asserts the coordinator sequenced
// an ingest and a rollout through the creator, the creator drove the deployment API,
// the request reconfigured, and the authority boundary held: no running-agent
// endpoint or credential is submitted through the flow (srd004/srd005, rel05.0
// uc001). It skips only if Go cannot build the agent; the live grounded-turn tier
// (rel05.0 S5) rides on Integration.Chatbot and the deploy swap.
func (Integration) ControlPlane() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(profilesRoot, "agent-core"))
	if err := requireProfilePaths(profilesRoot,
		"agents/coordinator/profile.yaml", "agents/creator/profile.yaml",
		"agents/chatbot/rest.yaml",
	); err != nil {
		return err
	}
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP controlPlane: agent-core checkout not found at %s (set %s)\n", coreRoot, agentCoreRootEnv)
		return nil
	}
	return runControlPlaneIntegration(profilesRoot, coreRoot)
}

func runControlPlaneIntegration(profilesRoot, coreRoot string) error {
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(binary) }()

	// The fake deployment API records the creator's calls so the test can assert the
	// authority boundary. The creator's default DEPLOYMENT_API_URL is this address.
	rec := &deploymentAPIRecorder{}
	stopAPI, err := startFakeDeploymentAPI(rec)
	if err != nil {
		return fmt.Errorf("controlPlane requires fake deployment API: %w", err)
	}
	defer stopAPI()

	// Start the creator first (the coordinator delegates to it). Its default
	// CREATOR_URL/DEPLOYMENT_API_URL reach this test's ports, so no env is needed.
	creatorTrace, creatorCleanup, err := chromaTraceFile("controlplane-creator")
	if err != nil {
		return err
	}
	defer creatorCleanup()
	stopCreator, err := startDetachedAgent(binary, profilesRoot, coreRoot, "agents/creator/profile.yaml", creatorTrace)
	if err != nil {
		return err
	}
	creatorStopped := false
	defer func() {
		if !creatorStopped {
			_ = stopCreator(true)
		}
	}()
	if err := waitHTTPStatus(cpCreatorControl, http.StatusOK, 30*time.Second); err != nil {
		return fmt.Errorf("creator control health never came up: %w", err)
	}

	coordTrace, coordCleanup, err := chromaTraceFile("controlplane-coordinator")
	if err != nil {
		return err
	}
	defer coordCleanup()
	stopCoord, err := startDetachedAgent(binary, profilesRoot, coreRoot, "agents/coordinator/profile.yaml", coordTrace)
	if err != nil {
		return err
	}
	coordStopped := false
	defer func() {
		if !coordStopped {
			_ = stopCoord(true)
		}
	}()
	if err := waitHTTPStatus(cpCoordinatorControl, http.StatusOK, 30*time.Second); err != nil {
		return fmt.Errorf("coordinator control health never came up: %w", err)
	}

	// Drive the intent the chatbot's provisioning panel does: a declared-client POST
	// to the coordinator, carrying the full desired mesh state as a values-plane
	// document (srd004 R3.1) and no host, URL, or credential (srd002 R5.1).
	intent := `{"values":"{\"ragUnits\":[{\"name\":\"rag0\",\"collection\":\"corpus\"},{\"name\":\"rag2\",\"collection\":\"corpus2\"}]}","rag_name":"rag2","collection":"corpus2","embedding_model":"qwen3-embedding:8b","directory":"/corpus/new"}`
	// The coordinator answers the intent by driving a model-backed machine, so
	// this is inference work behind an HTTP call, not a probe (GH-709 R2).
	data, status, err := requestInference(http.MethodPost, cpCoordinatorProvision, intent, "coordinator provision intent")
	if err != nil {
		return fmt.Errorf("provision intent request failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("provision intent status = %d, want 200: %s", status, data)
	}
	var resp struct {
		Status string `json:"status"`
		Trace  struct {
			Status string `json:"status"`
		} `json:"trace"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("decode provision response: %w: %s", err, data)
	}
	if resp.Status != "reconfigured" || resp.Trace.Status != "succeeded" {
		return fmt.Errorf("provision response = %s, want status reconfigured / trace succeeded", data)
	}

	// The flow reached the creator, which drove the deployment API: the fake saw both
	// an apply (from the ingest-then-reconfigure sequence) and a rollout read.
	if got := rec.applyCount(); got < 1 {
		return fmt.Errorf("the creator did not drive the deployment-API apply path (apply count %d)", got)
	}
	if got := rec.rolloutCount(); got < 1 {
		return fmt.Errorf("the creator did not read the deployment-API rollout (rollout count %d)", got)
	}
	// Authority boundary (srd005 R5.3, srd004 R4.3): no request through the flow
	// carried an Authorization header or an endpoint-authority field.
	if rec.sawAuthHeader() {
		return fmt.Errorf("a deployment-API call carried an Authorization header; the declarative runtime holds no credential and must send none")
	}
	if field := rec.endpointAuthorityField(); field != "" {
		return fmt.Errorf("a deployment-API request body carried a transport-authority field %q; no endpoint may cross the boundary", field)
	}

	// The provisioning panel's initial mesh-view load (srd006 R1.5, GH-753): a
	// GET through the coordinator, which asks the creator, which reads the
	// deployment API's state surface. Live evidence that the flat
	// executor -> creator -> coordinator field mapping actually works end to
	// end, not just that the YAML declares it.
	stateData, stateStatus, err := requestInference(http.MethodGet, cpCoordinatorState, "", "coordinator state read")
	if err != nil {
		return fmt.Errorf("state read request failed: %w", err)
	}
	if stateStatus != http.StatusOK {
		return fmt.Errorf("state read status = %d, want 200: %s", stateStatus, stateData)
	}
	var stateResp struct {
		SchemaVersion string `json:"schema_version"`
		Rags          []struct {
			Name string `json:"name"`
		} `json:"rags"`
		LLMInCluster        bool   `json:"llmInCluster"`
		ParamsRouterDefault string `json:"paramsRouterDefault"`
	}
	if err := json.Unmarshal(stateData, &stateResp); err != nil {
		return fmt.Errorf("decode state response: %w: %s", err, stateData)
	}
	if stateResp.SchemaVersion != "1" {
		return fmt.Errorf("state schema_version = %q, want 1: %s", stateResp.SchemaVersion, stateData)
	}
	if len(stateResp.Rags) == 0 {
		return fmt.Errorf("state carries no rags; the fake deployment API's topology did not survive the hop: %s", stateData)
	}
	if stateResp.ParamsRouterDefault == "" {
		return fmt.Errorf("state carries no paramsRouterDefault; a flat field was dropped somewhere in the chain: %s", stateData)
	}
	if got := rec.stateCount(); got < 1 {
		return fmt.Errorf("the creator did not read the deployment-API state surface (state count %d)", got)
	}

	// Exit both agents gracefully so their span logs flush.
	if _, s, err := requestHTTP(http.MethodPost, cpCoordinatorExit, `{"reason":"controlplane done"}`); err != nil || s/100 != 2 {
		return fmt.Errorf("coordinator exit failed: status %d: %v", s, err)
	}
	if err := stopCoord(false); err != nil {
		return fmt.Errorf("coordinator did not exit gracefully: %w", err)
	}
	coordStopped = true
	if _, s, err := requestHTTP(http.MethodPost, cpCreatorExit, `{"reason":"controlplane done"}`); err != nil || s/100 != 2 {
		return fmt.Errorf("creator exit failed: status %d: %v", s, err)
	}
	if err := stopCreator(false); err != nil {
		return fmt.Errorf("creator did not exit gracefully: %w", err)
	}
	creatorStopped = true

	fmt.Println("integration:controlPlane PASS - the intent flowed chatbot->coordinator->creator->deployment API, the creator applied and health-checked the reconfiguration, the request reconfigured, and no endpoint or credential crossed the authority boundary")
	return nil
}

// deploymentAPIRecorder records what the creator sends to the deployment API so the
// test can assert the authority boundary.
type deploymentAPIRecorder struct {
	mu       sync.Mutex
	applies  int
	rollouts int
	states   int
	authSeen bool
	badField string
}

var transportAuthorityFields = []string{"host", "url", "method", "token", "credential", "authorization", "endpoint", "base_url"}

func (r *deploymentAPIRecorder) record(req *http.Request, body map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.Header.Get("Authorization") != "" {
		r.authSeen = true
	}
	for _, f := range transportAuthorityFields {
		if _, ok := body[f]; ok && r.badField == "" {
			r.badField = f
		}
	}
}

func (r *deploymentAPIRecorder) applyCount() int { r.mu.Lock(); defer r.mu.Unlock(); return r.applies }
func (r *deploymentAPIRecorder) rolloutCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rollouts
}
func (r *deploymentAPIRecorder) stateCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.states
}
func (r *deploymentAPIRecorder) sawAuthHeader() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.authSeen
}
func (r *deploymentAPIRecorder) endpointAuthorityField() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.badField
}

// startFakeDeploymentAPI binds the deployment API's default address (:18090, the
// executor's apply port) and answers the apply and rollout paths the creator drives,
// recording each call. It returns a
// stop function, or an error if the port is already bound.
func startFakeDeploymentAPI(rec *deploymentAPIRecorder) (func(), error) {
	return startFakeDeploymentAPIOnAddr(rec, cpDeploymentAPIAddr)
}

func startFakeDeploymentAPIOnAddr(rec *deploymentAPIRecorder, address string) (func(), error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("bind fake deployment API on %s: %w", address, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/provisioning/api/apply", func(w http.ResponseWriter, req *http.Request) {
		var body map[string]interface{}
		data, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(data, &body)
		rec.record(req, body)
		rec.mu.Lock()
		rec.applies++
		rec.mu.Unlock()
		// Mirror the executor's versioned apply response (srd006 R1.4): a
		// schema_version-tagged status. Strict request validation (schema_version +
		// content required, helm dry-run) is proven against the real executor by the
		// integration:executor tracer (#602); here the fake records the call.
		writeJSON(w, map[string]interface{}{"schema_version": "1", "status": "applied"})
	})
	mux.HandleFunc("/provisioning/api/rollout", func(w http.ResponseWriter, req *http.Request) {
		rec.record(req, nil)
		rec.mu.Lock()
		rec.rollouts++
		rec.mu.Unlock()
		// Mirror the executor's trimmed rollout response (srd006 R1.4): schema_version
		// and phase only.
		writeJSON(w, map[string]interface{}{"schema_version": "1", "phase": "complete"})
	})
	mux.HandleFunc("/provisioning/api/state", func(w http.ResponseWriter, req *http.Request) {
		rec.record(req, nil)
		rec.mu.Lock()
		rec.states++
		rec.mu.Unlock()
		// Mirror the executor's flat state_response (srd006 deployment_api_contract,
		// GH-752/GH-753): one selector per named field, so the fake sends what a real
		// executor's helm_get_values read would produce.
		writeJSON(w, map[string]interface{}{
			"schema_version":      "1",
			"rags":                []map[string]interface{}{{"name": "rag0", "collection": "corpus", "embeddingModel": "qwen3-embedding:8b", "replicas": 1}},
			"llmInCluster":        true,
			"llmExternalURL":      "http://ollama.default.svc.cluster.local:11434",
			"llmChatModel":        "qwen2.5:3b",
			"llmEmbedModel":       "qwen3-embedding:8b",
			"llmChatModels":       []string{"qwen2.5:3b", "ornith:9b"},
			"llmRouterModel":      "qwen2.5:3b",
			"llmTopology":         "single",
			"paramsNResults":      5,
			"paramsChunkCap":      0,
			"paramsRouterDefault": "invoke_llm_fast",
		})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	return func() {
		_ = srv.Close()
		_ = listener.Close()
	}, nil
}

func writeJSON(w http.ResponseWriter, obj map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(obj)
	_, _ = w.Write(data)
}

// controlPlaneBodyIsClean reports whether a request body carries no transport
// -authority field, the check the recorder applies to every deployment-API call.
func controlPlaneBodyIsClean(body map[string]interface{}) bool {
	for _, f := range transportAuthorityFields {
		if _, ok := body[f]; ok {
			return false
		}
	}
	return true
}
