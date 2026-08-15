// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	cpProvisioningWorkflowOrchestratorControl = "http://127.0.0.1:18101/api/lifecycle/health"
	cpProvisioningWorkflowOrchestratorExit    = "http://127.0.0.1:18101/api/lifecycle/exit"
	cpProvisioningWorkflowOrchestratorApply   = "http://127.0.0.1:18100/provisioning/api/apply"
	cpProvisioningWorkflowOrchestratorState   = "http://127.0.0.1:18100/provisioning/api/state"
	cpCreatorControl                          = "http://127.0.0.1:18111/api/lifecycle/health"
	cpCreatorExit                             = "http://127.0.0.1:18111/api/lifecycle/exit"
	cpDeploymentAPIAddr                       = "127.0.0.1:18090"
)

// ControlPlane proves the realized mesh control-plane flow end to end without a
// cluster: an operator's values-apply intent flows panel -> provisioning-workflow-orchestrator -> creator
// -> deployment API. The provisioning-workflow-orchestrator and creator run as the real declarative
// agents; a fake deployment API stands in for the applier (srd006) and records
// what the creator sends. The test drives the panel's apply the way the SPA does --
// a POST to the provisioning-workflow-orchestrator's /provisioning/api/apply -- then asserts the
// provisioning-workflow-orchestrator delegated the rollout to the creator, the creator drove the
// deployment API and verified health, the request reconfigured, the panel's mesh
// view reads back through both hops, and the authority boundary held: no
// running-agent endpoint or credential is submitted through the flow
// (srd004/srd005). It skips only if Go cannot build the agent; the live
// grounded-turn tier rides on Integration.Chatbot and the deploy swap.
//
// What this does NOT drive is the provisioning-workflow-orchestrator's other intake,
// POST /api/v1/provision, whose Seed leg ingests a directory before reconfiguring.
// The creator now realizes srd005 R3.1 through its dedicated /api/v1/ingest
// endpoint and SeedIngest leg, including pre/post collection-count checks. This
// tracer remains scoped to the values-only path: it does not invoke that ingest
// leg or prove one live lifecycle joining ingest, rollout, and a grounded turn
// from the newly added source.
func (Integration) ControlPlane() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(profilesRoot)
	if err := requireProfilePaths(profilesRoot,
		"agents/provisioning-workflow-orchestrator/profile.yaml", "agents/creator/profile.yaml",
		"agents/chatbot/rest.yaml",
	); err != nil {
		return err
	}
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP controlPlane: agent-core checkout not found at %s (set core_root in demo.yaml)\n", coreRoot)
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

	// The fake deployment API records the creator's calls so the test can assert
	// the authority boundary. It owns an ephemeral loopback port rather than the
	// production applier default (:18090), so ambient listeners and other live
	// integrations cannot make this proof fail (GH-1494).
	rec := &deploymentAPIRecorder{}
	stopAPI, deploymentAPIURL, err := startFakeDeploymentAPI(rec)
	if err != nil {
		return fmt.Errorf("controlPlane requires fake deployment API: %w", err)
	}
	defer stopAPI()
	_, deploymentAPIPort, err := net.SplitHostPort(
		strings.TrimPrefix(deploymentAPIURL, "http://"))
	if err != nil {
		return fmt.Errorf("controlPlane fake deployment API URL %q: %w",
			deploymentAPIURL, err)
	}

	// Start the creator first (the provisioning-workflow-orchestrator delegates
	// to it). The test supplies the fake URL through the creator's declared
	// DEPLOYMENT_API_URL contract; the shipped default remains unchanged.
	creatorTrace, creatorCleanup, err := chromaTraceFile("controlplane-creator")
	if err != nil {
		return err
	}
	defer creatorCleanup()
	stopCreator, err := startDetachedAgentWithEnv(agentLaunch{
		Binary: binary, ProfilesRoot: profilesRoot, CoreRoot: coreRoot,
		Profile: "agents/creator/profile.yaml", TracePath: creatorTrace,
		Env: []string{
			"DEPLOYMENT_API_URL=" + deploymentAPIURL,
			"DEPLOYMENT_API_PORT=" + deploymentAPIPort,
		},
		GracefulWait: 15 * time.Second,
	})
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

	coordTrace, coordCleanup, err := chromaTraceFile("controlplane-provisioning-workflow-orchestrator")
	if err != nil {
		return err
	}
	defer coordCleanup()
	stopCoord, err := startDetachedAgent(binary, profilesRoot, coreRoot, "agents/provisioning-workflow-orchestrator/profile.yaml", coordTrace)
	if err != nil {
		return err
	}
	coordStopped := false
	defer func() {
		if !coordStopped {
			_ = stopCoord(true)
		}
	}()
	if err := waitHTTPStatus(cpProvisioningWorkflowOrchestratorControl, http.StatusOK, 30*time.Second); err != nil {
		return fmt.Errorf("provisioning-workflow-orchestrator control health never came up: %w", err)
	}

	// Drive the intent the chatbot's provisioning panel does: the operator's
	// values-apply, a POST to the provisioning-workflow-orchestrator's browser-facing apply endpoint
	// carrying the full desired mesh state as a values-plane document
	// (srd004 R3.1) and no host, URL, or credential (srd002 R5.1). The endpoint
	// seeds SeedValues, so the run takes the reconfigure leg -- the path the mesh
	// actually realizes (see the note on ControlPlane about the ingest leg).
	intent := `{"values":"{\"ragUnits\":[{\"name\":\"rag0\",\"collection\":\"corpus\"},{\"name\":\"rag2\",\"collection\":\"corpus2\"}]}"}`
	// The provisioning-workflow-orchestrator answers the intent by driving a model-backed machine, so
	// this is inference work behind an HTTP call, not a probe (GH-709 R2).
	data, status, err := requestInference(http.MethodPost, cpProvisioningWorkflowOrchestratorApply, intent, "provisioning-workflow-orchestrator apply intent")
	if err != nil {
		return fmt.Errorf("apply intent request failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf(
			"apply intent status = %d, want 200: %s (deployment API %s calls apply=%d rollout=%d state=%d)",
			status, data, deploymentAPIURL,
			rec.applyCount(), rec.rolloutCount(), rec.stateCount())
	}
	var resp struct {
		Status string `json:"status"`
		Trace  struct {
			Status string `json:"status"`
		} `json:"trace"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("decode apply response: %w: %s", err, data)
	}
	if resp.Status != "reconfigured" || resp.Trace.Status != "succeeded" {
		return fmt.Errorf("apply response = %s, want status reconfigured / trace succeeded", data)
	}

	// The flow reached the creator, which drove the deployment API: the fake saw
	// the values apply and the rollout read that verifies it.
	if got := rec.applyCount(); got < 1 {
		return fmt.Errorf("the creator did not drive the deployment-API apply path (apply count %d)", got)
	}
	// The decided values document itself crossed the boundary, not merely a call.
	// Counting applies alone would pass on an empty or dropped content field,
	// which is exactly the failure mode that let the ingest leg look green while
	// forwarding nothing (GH-755).
	if content := rec.appliedContent(); !strings.Contains(content, "rag2") {
		return fmt.Errorf("the apply carried content %q, which does not contain the decided topology; the values document did not survive the provisioning-workflow-orchestrator -> creator hop", content)
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
	// GET through the provisioning-workflow-orchestrator, which asks the creator, which reads the
	// deployment API's state surface. Live evidence that the flat
	// applier -> creator -> provisioning-workflow-orchestrator field mapping actually works end to
	// end, not just that the YAML declares it.
	stateData, stateStatus, err := requestInference(http.MethodGet, cpProvisioningWorkflowOrchestratorState, "", "provisioning-workflow-orchestrator state read")
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
		LLMInCluster      bool   `json:"llmInCluster"`
		ParamsTierDefault string `json:"paramsTierDefault"`
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
	if stateResp.ParamsTierDefault == "" {
		return fmt.Errorf("state carries no paramsTierDefault; a flat field was dropped somewhere in the chain: %s", stateData)
	}
	if got := rec.stateCount(); got < 1 {
		return fmt.Errorf("the creator did not read the deployment-API state surface (state count %d)", got)
	}

	// Exit both agents gracefully so their span logs flush.
	if _, s, err := requestHTTP(http.MethodPost, cpProvisioningWorkflowOrchestratorExit, `{"reason":"controlplane done"}`); err != nil || s/100 != 2 {
		return fmt.Errorf("provisioning-workflow-orchestrator exit failed: status %d: %v", s, err)
	}
	if err := stopCoord(false); err != nil {
		return fmt.Errorf("provisioning-workflow-orchestrator did not exit gracefully: %w", err)
	}
	coordStopped = true
	if _, s, err := requestHTTP(http.MethodPost, cpCreatorExit, `{"reason":"controlplane done"}`); err != nil || s/100 != 2 {
		return fmt.Errorf("creator exit failed: status %d: %v", s, err)
	}
	if err := stopCreator(false); err != nil {
		return fmt.Errorf("creator did not exit gracefully: %w", err)
	}
	creatorStopped = true

	fmt.Println("integration:controlPlane PASS - the panel's values apply flowed provisioning-workflow-orchestrator->creator->deployment API carrying the decided document, the creator applied and health-checked the reconfiguration, the panel's mesh view read back through both hops, and no endpoint or credential crossed the authority boundary")
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
	// content is the values-plane document the last apply carried, so the test can
	// assert the decided values reached the applier rather than only that a call
	// was made.
	content string
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
func (r *deploymentAPIRecorder) appliedContent() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.content
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

// startFakeDeploymentAPI binds an isolated loopback port and returns the URL the
// creator under test must use. The production default remains :18090.
func startFakeDeploymentAPI(rec *deploymentAPIRecorder) (func(), string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("bind dynamic fake deployment API: %w", err)
	}
	stop := serveFakeDeploymentAPI(rec, listener)
	return stop, "http://" + listener.Addr().String(), nil
}

func startFakeDeploymentAPIOnAddr(rec *deploymentAPIRecorder, address string) (func(), error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("bind fake deployment API on %s: %w", address, err)
	}
	return serveFakeDeploymentAPI(rec, listener), nil
}

func serveFakeDeploymentAPI(rec *deploymentAPIRecorder, listener net.Listener) func() {
	mux := http.NewServeMux()
	mux.HandleFunc("/provisioning/api/apply", func(w http.ResponseWriter, req *http.Request) {
		var body map[string]interface{}
		data, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(data, &body)
		rec.record(req, body)
		rec.mu.Lock()
		rec.applies++
		if content, ok := body["content"].(string); ok {
			rec.content = content
		}
		rec.mu.Unlock()
		// Mirror the applier's versioned apply response (srd006 R1.4): a
		// schema_version-tagged status. Strict request validation (schema_version +
		// content required, helm dry-run) is proven against the real applier by the
		// integration:applier tracer (#602); here the fake records the call.
		writeJSON(w, map[string]interface{}{"schema_version": "1", "status": "applied"})
	})
	mux.HandleFunc("/provisioning/api/rollout", func(w http.ResponseWriter, req *http.Request) {
		rec.record(req, nil)
		rec.mu.Lock()
		rec.rollouts++
		rec.mu.Unlock()
		// Mirror the applier's trimmed rollout response (srd006 R1.4): schema_version
		// and phase only.
		writeJSON(w, map[string]interface{}{"schema_version": "1", "phase": "complete"})
	})
	mux.HandleFunc("/provisioning/api/state", func(w http.ResponseWriter, req *http.Request) {
		rec.record(req, nil)
		rec.mu.Lock()
		rec.states++
		rec.mu.Unlock()
		// Mirror the applier's flat state_response (srd006 deployment_api_contract,
		// GH-752/GH-753): one selector per named field, so the fake sends what a real
		// applier's helm_get_values read would produce.
		writeJSON(w, map[string]interface{}{
			"schema_version":    "1",
			"rags":              []map[string]interface{}{{"name": "rag0", "collection": "corpus", "embeddingModel": "qwen3-embedding:8b", "replicas": 1}},
			"llmInCluster":      true,
			"llmExternalURL":    "http://ollama.default.svc.cluster.local:11434",
			"llmChatModel":      "qwen2.5:3b",
			"llmEmbedModel":     "qwen3-embedding:8b",
			"llmChatModels":     []string{"qwen2.5:3b", "ornith:9b"},
			"llmTierModel":      "qwen2.5:3b",
			"llmTopology":       "single",
			"paramsNResults":    5,
			"paramsChunkCap":    0,
			"paramsTierDefault": "invoke_llm_fast",
		})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	return func() {
		_ = srv.Close()
		_ = listener.Close()
	}
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
