// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBenchConformance launches the bench profile, waits for its generic REST
// health route, and posts a shutdown action so the host machine drains the
// profile-owned listener and reaches Done.
//
// It runs the wrapper an operator ships — agents/bench/profile.yaml — through a
// temp copy, patching only the profile REST listen address in rest.yaml
// so the UI host does not collide with a real bench server on :8080. The
// profile's /opt/agent-core tool_config_dir remaps onto the checkout via
// --core-root; nothing else is rebuilt.
//
// The generic REST event queue is the human input boundary. The Serving -> Done
// path needs no evaluator launch, so this test drives only shutdown.
func TestBenchConformance(t *testing.T) {
	RequireCoreRoot(t)
	addr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "bench", "profile.yaml"), map[string]string{
		"address: 127.0.0.1:8080": `address: ` + addr,
	})
	runDir := t.TempDir()
	child := filepath.Join(runDir, "critic-child")
	capture := filepath.Join(runDir, "child-args.txt")
	if err := os.WriteFile(child, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$BENCH_CHILD_ARGS\"\n"), 0o755); err != nil {
		t.Fatalf("write critic child: %v", err)
	}

	server := Serve(t, ServeConfig{
		Profile: profilePath, Directory: ProfilesRoot(),
		Args: []string{"--child-agent-binary", child},
		Env:  []string{"BENCH_CHILD_ARGS=" + capture},
	})
	server.WaitHealthy("http://"+addr+"/api/v1/health", 15*time.Second)
	requireBenchResponse(t, "http://"+addr+"/", http.StatusOK, "<div id=\"root\"></div>")
	requireBenchResponse(t, "http://"+addr+"/api/v1/sessions", http.StatusOK, `"data":[]`)
	requireBenchResponse(t, "http://"+addr+"/api/v1/configs", http.StatusOK, `"category"`)
	requireBenchResponse(t, "http://"+addr+"/api/v1/configs/bench/machine.yaml", http.StatusOK, `"graph"`)
	requireBenchResponse(t, "http://"+addr+"/api/v1/source/agents/bench/machine.yaml", http.StatusOK, `"language":"yaml"`)
	if status := server.Post("http://"+addr+"/api/v1/actions", `{"type":"launch_eval","config":{"suite":"suites/basic.yaml","output_dir":"eval-results"}}`); status != http.StatusAccepted {
		t.Fatalf("launch action POST status = %d, want %d", status, http.StatusAccepted)
	}
	requireBenchChildArgs(t, capture)
	if status := server.Post("http://"+addr+"/api/v1/actions", `{"type":"shutdown"}`); status != http.StatusAccepted {
		t.Fatalf("shutdown action POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(15 * time.Second)

	// srd006: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RequireNoErrorSpans(t)

	// srd006: generic REST lifecycle words are the visible human-input boundary.
	result.RequireToolSpans(t, "launch_bench_http", "await_bench_action", "stop_bench_http")
	result.RequireToolSpans(t, "list_evaluation_sessions", "list_resource", "read_resource")
	result.RequireToolSpans(t, "validate_eval_suite", "self_invoke")

	// srd006: the host shutdown reaches Done even though request machines also
	// contribute terminal events to the shared trace.
	requireBenchTerminalState(t, result, "Done")
}

func requireBenchChildArgs(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			text := string(data)
			for _, want := range []string{
				"--profile\nagents/critic/profile.yaml",
				"--request\nsuites/basic.yaml",
				"--output\neval-results",
			} {
				if !strings.Contains(text, want) {
					t.Fatalf("critic child args missing %q:\n%s", want, text)
				}
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("critic child did not record args at %s", path)
}

func requireBenchResponse(t *testing.T, url string, status int, contains string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", url, err)
	}
	if response.StatusCode != status || !strings.Contains(string(body), contains) {
		t.Fatalf("GET %s = %d %s, want %d containing %q", url, response.StatusCode, body, status, contains)
	}
}

func requireBenchTerminalState(t *testing.T, result RunResult, want string) {
	t.Helper()
	for _, span := range result.Spans {
		for _, event := range span.Events {
			if event.Name != TerminalEventName {
				continue
			}
			if state, _ := event.StringAttr("final_state"); state == want {
				return
			}
		}
	}
	t.Fatalf("no terminal event reached %q; spans: %v", want, result.Spans.Names())
}
