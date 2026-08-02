// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareDocumentationCuratorIntegrationWritesEphemeralProfile(t *testing.T) {
	profilesRoot := t.TempDir()
	coreRoot := t.TempDir()
	writeDocumentationCuratorFixture(t, profilesRoot)
	writeFile(t, filepath.Join(coreRoot, "tools", "builtin", "lifecycle", "exit-agent.yaml"), "tools: []\n")
	writeFile(t, filepath.Join(coreRoot, "tools", "builtin", "spec-validation", "all.yaml"), "tools: []\n")
	writeFile(t, filepath.Join(coreRoot, "docs", "SPECIFICATIONS.yaml"), "id: specs\n")
	writeFile(t, filepath.Join(coreRoot, "configs", "sample.yaml"), "id: sample\n")
	writeFile(t, filepath.Join(coreRoot, "unrelated-dirty-file"), "preserve me\n")

	cfg, cleanup, err := prepareDocumentationCuratorIntegration(profilesRoot, coreRoot)
	if err != nil {
		t.Fatalf("prepareDocumentationCuratorIntegration: %v", err)
	}

	tmpDir := filepath.Dir(cfg.profilePath)
	profile := readTestFile(t, cfg.profilePath)
	for _, want := range []string{
		filepath.Join(tmpDir, "builtin.yaml"),
		filepath.Join(tmpDir, "rest.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "lifecycle", "exit-agent.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "spec-validation", "all.yaml"),
	} {
		if !strings.Contains(profile, want) {
			t.Fatalf("profile missing %q:\n%s", want, profile)
		}
	}

	rest := readTestFile(t, filepath.Join(tmpDir, "rest.yaml"))
	for _, absent := range []string{"18081", "18082", "18084"} {
		if strings.Contains(rest, absent) {
			t.Fatalf("rest.yaml still contains fixed port %s:\n%s", absent, rest)
		}
	}
	if !strings.Contains(rest, "ports: ["+localPort(cfg.docsAddr)+"]") {
		t.Fatalf("rest.yaml missing docs port %s:\n%s", cfg.docsAddr, rest)
	}
	if !strings.Contains(rest, "address: "+cfg.controlAddr) {
		t.Fatalf("rest.yaml missing control address %s:\n%s", cfg.controlAddr, rest)
	}
	if !strings.Contains(rest, "address: "+cfg.docsAddr) {
		t.Fatalf("rest.yaml missing docs address %s:\n%s", cfg.docsAddr, rest)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "ui", "ux.yaml")); err != nil {
		t.Fatalf("expected copied UX config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.workspace, "docs", "SPECIFICATIONS.yaml")); err != nil {
		t.Fatalf("expected isolated documentation corpus: %v", err)
	}
	if cfg.workspace == coreRoot {
		t.Fatal("documentation-curator workspace must not be the agent-core checkout")
	}

	writeFile(t, filepath.Join(cfg.workspace, ".documentation-curator", "patches", "test.json"), "{}")
	if _, err := os.Stat(filepath.Join(coreRoot, ".documentation-curator")); !os.IsNotExist(err) {
		t.Fatalf("agent-core checkout contains leaked curator state: %v", err)
	}
	cleanup()
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("temporary curator state survived cleanup: %v", err)
	}
	if got := readTestFile(t, filepath.Join(coreRoot, "unrelated-dirty-file")); got != "preserve me\n" {
		t.Fatalf("cleanup changed unrelated checkout state: %q", got)
	}
}

func TestPrepareDocumentationCuratorIntegrationCleansUpAfterFailure(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)
	profilesRoot := t.TempDir()
	coreRoot := t.TempDir()
	writeDocumentationCuratorFixture(t, profilesRoot)

	_, _, err := prepareDocumentationCuratorIntegration(profilesRoot, coreRoot)
	if err == nil {
		t.Fatal("expected missing docs directory to fail preparation")
	}
	entries, readErr := os.ReadDir(tmpRoot)
	if readErr != nil {
		t.Fatalf("read temporary root: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed preparation leaked temporary state: %v", entries)
	}
}

func TestRequireDocumentTraceAcceptsMachineRequestTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"trace":{"server":"documentation_curator","route":"document","machine":"documentation-curator-request","terminal_signal":"DocumentDetailReady","status":"succeeded"}}`))
	}))
	defer server.Close()

	if err := requireDocumentTrace(strings.TrimPrefix(server.URL, "http://"), "/api/v1/docs/SPECIFICATIONS.yaml"); err != nil {
		t.Fatalf("requireDocumentTrace returned error: %v", err)
	}
}

func TestRequireDocumentTraceRejectsMissingTraceFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"trace":{"terminal_signal":"DocumentDetailReady"}}`))
	}))
	defer server.Close()

	err := requireDocumentTrace(strings.TrimPrefix(server.URL, "http://"), "/api/v1/docs/SPECIFICATIONS.yaml")
	if err == nil {
		t.Fatal("expected missing trace field error")
	}
	if !strings.Contains(err.Error(), `trace missing "server"`) {
		t.Fatalf("error = %q, want missing server field", err)
	}
}

func writeDocumentationCuratorFixture(t *testing.T, root string) {
	t.Helper()
	base := filepath.Join(root, documentationCuratorProfile)
	writeFile(t, filepath.Join(base, "machine.yaml"), "name: documentation-curator\n")
	writeFile(t, filepath.Join(base, "tools.yaml"), "tools: []\n")
	writeFile(t, filepath.Join(base, "declarations.yaml"), "tools: []\n")
	writeFile(t, filepath.Join(base, "request-declarations.yaml"), "tools: []\n")
	writeFile(t, filepath.Join(base, "request-machine.yaml"), "name: request\n")
	writeFile(t, filepath.Join(base, "openapi.yaml"), "servers:\n  - url: http://127.0.0.1:18081\n")
	writeFile(t, filepath.Join(base, "ui", "ux.yaml"), "id: documentation-curator-ui\n")
	writeFile(t, filepath.Join(base, "builtin.yaml"), `tools:
  - name: launch_curator_http
    init: rest_server_launch
    config:
      rest_ref: documentation_curator
`)
	writeFile(t, filepath.Join(base, "rest.yaml"), `rest:
  openapi:
    documentation_curator:
      path: openapi.yaml
      base_url: http://127.0.0.1:18081
  limits:
    local_docs_api:
      network:
        ports: [18081]
    local_control_api:
      network:
        ports: [18082]
    local_machine_request_api:
      network:
        ports: [18084]
  servers:
    documentation_curator:
      address: 127.0.0.1:18081
    documentation_curator_control:
      address: 127.0.0.1:18082
    monitor:
      address: 127.0.0.1:18084
`)
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
