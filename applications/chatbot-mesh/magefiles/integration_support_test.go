// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWaitHTTPStatusBoundsStalledRequestByOuterDeadline(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		<-req.Context().Done()
	}))
	defer server.Close()

	start := time.Now()
	err := waitHTTPStatusWithClient(&http.Client{}, server.URL, http.StatusOK, 100*time.Millisecond)

	if err == nil {
		t.Fatal("waitHTTPStatusWithClient() expected timeout")
	}
	if elapsed := time.Since(start); elapsed >= 500*time.Millisecond {
		t.Fatalf("waitHTTPStatusWithClient() elapsed %s, outer deadline was 100ms", elapsed)
	}
}

func TestWaitHTTPStatusPreservesLastTransportError(t *testing.T) {
	t.Parallel()
	transportErr := errors.New("injected transport failure")
	client := &http.Client{Transport: meshRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}

	err := waitHTTPStatusWithClient(client, "http://integration.invalid", http.StatusOK, 120*time.Millisecond)

	if !errors.Is(err, transportErr) {
		t.Fatalf("waitHTTPStatusWithClient() error = %v, want wrapped transport error", err)
	}
}

func TestWaitHTTPStatusReturnsOnExpectedStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	if err := waitHTTPStatusWithClient(&http.Client{}, server.URL, http.StatusAccepted, time.Second); err != nil {
		t.Fatalf("waitHTTPStatusWithClient() error: %v", err)
	}
}

func TestDetachedAgentCleanupReportsProcessOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		script      string
		forceKill   bool
		wait        time.Duration
		wantErrText string
	}{
		{name: "zero exit", script: "#!/bin/sh\nexit 0\n", wait: time.Second},
		{name: "spontaneous crash", script: "#!/bin/sh\nexit 7\n", wait: time.Second, wantErrText: "exit status 7"},
		{name: "expected force kill", script: "#!/bin/sh\nwhile :; do :; done\n", forceKill: true, wait: time.Second},
		{name: "graceful timeout", script: "#!/bin/sh\nwhile :; do :; done\n", wait: 20 * time.Millisecond, wantErrText: "did not stop within"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			binary := filepath.Join(root, "fake-agent")
			if err := os.WriteFile(binary, []byte(tt.script), 0o700); err != nil {
				t.Fatalf("write fake agent: %v", err)
			}
			stop, err := startDetachedAgentWithTimeout(binary, root, root, "profile.yaml", filepath.Join(root, "trace.json"), tt.wait)
			if err != nil {
				t.Fatalf("startDetachedAgentWithTimeout(): %v", err)
			}
			err = stop(tt.forceKill)
			if tt.wantErrText == "" {
				if err != nil {
					t.Fatalf("stop() error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("stop() error = %v, want text %q", err, tt.wantErrText)
			}
		})
	}
}

func TestDetachedAgentDualExportsAndStampsResourceIdentity(t *testing.T) {
	root := t.TempDir()
	argsPath := filepath.Join(root, "args.txt")
	attrsPath := filepath.Join(root, "attrs.txt")
	binary := filepath.Join(root, "fake-agent")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGS_FILE\"\nprintf '%s' \"$OTEL_RESOURCE_ATTRIBUTES\" > \"$ATTRS_FILE\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	stop, err := startDetachedAgentWithEnv(agentLaunch{
		Binary:       binary,
		ProfilesRoot: root,
		CoreRoot:     root,
		Profile:      "agents/rag-server/profile.yaml",
		TracePath:    filepath.Join(root, "trace.json"),
		OTLPEndpoint: "127.0.0.1:4317",
		ServiceName:  "rag0",
		Target:       "integration:chatbot",
		RunID:        "run-123",
		GitCommit:    "abc123",
		Env:          []string{"ARGS_FILE=" + argsPath, "ATTRS_FILE=" + attrsPath},
		GracefulWait: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(false); err != nil {
		t.Fatal(err)
	}
	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(argsData)
	for _, want := range []string{
		"--otel-log-file\n", "--otel-otlp-endpoint\n127.0.0.1:4317\n",
		"--otel-service-name\nrag0\n",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q:\n%s", want, args)
		}
	}
	attrsData, err := os.ReadFile(attrsPath)
	if err != nil {
		t.Fatal(err)
	}
	attrs := string(attrsData)
	for _, want := range []string{
		"test.repository=Nokia-Bell-Labs%2Fdeclarative-agents",
		"test.module=applications%2Fchatbot-mesh",
		"test.target=integration%3Achatbot",
		"vcs.ref.head.revision=abc123",
		"test.run.id=run-123",
	} {
		if !strings.Contains(attrs, want) {
			t.Errorf("resource attributes missing %q: %s", want, attrs)
		}
	}
}

func TestDetachedAgentKeepsFileOnlyArgsWithoutEndpoint(t *testing.T) {
	t.Setenv(integrationOTLPEndpointEnv, "")
	root := t.TempDir()
	argsPath := filepath.Join(root, "args.txt")
	binary := filepath.Join(root, "fake-agent")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGS_FILE\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	stop, err := startDetachedAgentWithEnv(agentLaunch{
		Binary: binary, ProfilesRoot: root, CoreRoot: root,
		Profile: "profile.yaml", TracePath: filepath.Join(root, "trace.json"),
		RunID: "run-123", GitCommit: "abc123",
		Env: []string{"ARGS_FILE=" + argsPath}, GracefulWait: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(false); err != nil {
		t.Fatal(err)
	}
	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(argsData)
	if strings.Contains(args, "--otel-otlp-endpoint") || strings.Contains(args, "--otel-service-name") {
		t.Fatalf("file-only launch gained live-export args:\n%s", args)
	}
	if !strings.Contains(args, "--otel-log-file") {
		t.Fatalf("file-only launch lost trace file arg:\n%s", args)
	}
}

type meshRoundTripFunc func(*http.Request) (*http.Response, error)

func (f meshRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
