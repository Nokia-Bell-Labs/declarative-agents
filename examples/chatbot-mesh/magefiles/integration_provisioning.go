// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Provisioning drives the deployment API (srd003 R4) end to end against the real
// provisioner binary: it reads the deployed mesh view, applies an add-RAG values
// patch, and reads the rollout, exactly as the provisioning panel does over HTTP.
// The apply path's helm upgrade and the rollout's kubectl are stubbed with fake
// binaries on PATH, so the test proves the HTTP, auth (R4.1), and values-patch
// contract without a cluster; the on-cluster rollout is Integration.HelmSwap and
// the panel-served mesh. It skips only if Go cannot build the service.
func (Integration) Provisioning() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	provDir := filepath.Join(profilesRoot, "provisioner")
	if _, err := os.Stat(filepath.Join(provDir, "go.mod")); err != nil {
		fmt.Printf("SKIP provisioning: provisioner module not found at %s\n", provDir)
		return nil
	}
	return runProvisioningIntegration(provDir)
}

func runProvisioningIntegration(provDir string) error {
	workDir, err := os.MkdirTemp("", "provisioner-it-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	binary := filepath.Join(workDir, "provisioner")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = provDir
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build provisioner: %w", err)
	}

	// A one-RAG deployed mesh view (what the chart's mesh-state ConfigMap holds).
	stateFile := filepath.Join(workDir, "mesh.json")
	if err := os.WriteFile(stateFile, []byte(`{"rags":[{"name":"rag0","collection":"corpus","embeddingModel":"qwen3-embedding:8b","replicas":1}],"llm":{"externalURL":"http://ollama:11434","embedModel":"qwen3-embedding:8b"},"params":{"nResults":5,"chunkCap":0,"routerDefault":"invoke_llm_fast"}}`), 0o644); err != nil {
		return err
	}
	// Fake helm/kubectl so apply and rollout succeed without a cluster; the apply
	// records its invocation so the test proves the values patch reached helm.
	fakeBin := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		return err
	}
	helmLog := filepath.Join(workDir, "helm-args.txt")
	if err := writeExecutable(filepath.Join(fakeBin, "helm"), "#!/bin/sh\necho \"$@\" >> "+helmLog+"\nexit 0\n", "fake helm"); err != nil {
		return err
	}
	if err := writeExecutable(filepath.Join(fakeBin, "kubectl"), "#!/bin/sh\nprintf '2/2/3'\n", "fake kubectl"); err != nil {
		return err
	}

	addr, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	readTokenFile := filepath.Join(workDir, "read-token")
	applyTokenFile := filepath.Join(workDir, "apply-token")
	if err := os.WriteFile(readTokenFile, []byte("read-tok"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(applyTokenFile, []byte("apply-tok"), 0o600); err != nil {
		return err
	}
	cmd := exec.Command(binary,
		"--read-token-file="+readTokenFile,
		"--apply-token-file="+applyTokenFile,
	)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PROVISION_ADDR="+addr,
		"PROVISION_STATE_FILE="+stateFile,
		"PROVISION_CHART_DIR="+workDir,
	)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start provisioner: %w", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	base := "http://" + addr + "/provisioning/api"
	if err := waitHTTPStatus(base+"/health", http.StatusOK, 10*time.Second); err != nil {
		return fmt.Errorf("provisioner did not become ready: %w", err)
	}

	// Read path: the deployed mesh view, one RAG.
	state, err := provRequest(http.MethodGet, base+"/state", "read-tok", "")
	if err != nil {
		return err
	}
	var view struct {
		Rags []struct{ Name string } `json:"rags"`
	}
	if err := json.Unmarshal(state, &view); err != nil {
		return fmt.Errorf("decode state: %w", err)
	}
	if len(view.Rags) != 1 || view.Rags[0].Name != "rag0" {
		return fmt.Errorf("state = %s, want one rag0 unit", state)
	}

	// Authority boundary (R4.1): the read token must not apply.
	if _, err := provRequest(http.MethodPost, base+"/apply", "read-tok", twoRagPatch); err == nil {
		return fmt.Errorf("apply with the read token succeeded; it must be rejected (R4.1)")
	}

	// Apply an add-RAG values patch, as the panel's apply does.
	if _, err := provRequest(http.MethodPost, base+"/apply", "apply-tok", twoRagPatch); err != nil {
		return fmt.Errorf("add-RAG apply: %w", err)
	}
	helmArgs, err := os.ReadFile(helmLog)
	if err != nil {
		return fmt.Errorf("read helm invocation log: %w", err)
	}
	for _, want := range []string{"upgrade", "ragUnits[1].name=rag1"} {
		if !strings.Contains(string(helmArgs), want) {
			return fmt.Errorf("apply did not render the add-RAG helm upgrade (missing %q); got %s", want, helmArgs)
		}
	}

	// Rollout path: progress the panel polls.
	rollout, err := provRequest(http.MethodGet, base+"/rollout", "read-tok", "")
	if err != nil {
		return err
	}
	if !strings.Contains(string(rollout), "\"phase\":\"complete\"") {
		return fmt.Errorf("rollout = %s, want phase complete", rollout)
	}

	fmt.Println("integration:provisioning PASS - the deployment API read the mesh, rejected an apply from the read token, rendered the add-RAG helm upgrade, and reported rollout progress")
	return nil
}

const twoRagPatch = `{"rags":[{"name":"rag0","collection":"corpus","embeddingModel":"qwen3-embedding:8b","replicas":1},{"name":"rag1","collection":"corpus2","embeddingModel":"qwen3-embedding:8b","replicas":1}],"llm":{"externalURL":"http://ollama:11434","embedModel":"qwen3-embedding:8b"},"params":{"nResults":5,"chunkCap":0,"routerDefault":"invoke_llm_fast"}}`

// provRequest issues an authenticated deployment-API request and returns the body,
// erroring on a non-2xx status so the caller asserts the contract.
func provRequest(method, url, token, body string) ([]byte, error) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return data, fmt.Errorf("%s %s: status %d: %s", method, url, resp.StatusCode, data)
	}
	return data, nil
}
