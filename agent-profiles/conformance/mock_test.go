// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockFixture resolves one of the shipped example fixtures.
func mockFixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "testdata", "conformance", "mock", name))
	if err != nil {
		t.Fatalf("resolve fixture %s: %v", name, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return path
}

// mockLog reads the mock's request log route.
func mockLog(t *testing.T, baseURL string) []map[string]interface{} {
	t.Helper()
	resp, err := http.Get(baseURL + "/_mock/log")
	if err != nil {
		t.Fatalf("read mock log: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read mock log body: %v", err)
	}
	var payload struct {
		Count    int                      `json:"count"`
		Requests []map[string]interface{} `json:"requests"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode mock log %q: %v", string(data), err)
	}
	if payload.Count != len(payload.Requests) {
		t.Fatalf("mock log count %d != %d entries", payload.Count, len(payload.Requests))
	}
	return payload.Requests
}

func mockStatus(t *testing.T, method, url, body string) int {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// TestMockServesFixtureSequence runs the shipped mock profile against the
// scripted example fixture and asserts the srd039 serving contract end to end:
// declared order, the repeat once the script is exhausted, a 404 for an
// unmatched route, and a clean stop on the control event.
//
// It runs the wrapper an operator ships — agents/mock/profile.yaml — pointing
// the listener and the fixture at test values through the environment, which
// is the profile's own parameterization rather than a patched copy.
//
// Traces srd019-mock AC1 and AC2, and rel11.0-uc002-mock-serves-fixtures S2.
func TestMockServesFixtureSequence(t *testing.T) {
	RequireCoreRoot(t)
	addr := FreeAddr(t)
	baseURL := "http://" + addr

	server := Serve(t, ServeConfig{
		Profile: filepath.Join("agents", "mock", "profile.yaml"),
		Env: []string{
			"MOCK_ADDRESS=" + addr,
			"MOCK_FIXTURES=" + mockFixture(t, "scripted.yaml"),
		},
	})
	server.WaitHealthy(baseURL+"/_mock/health", 15*time.Second)

	// The script is 200 then 500; the last response repeats once exhausted.
	wantStatuses := []int{http.StatusOK, http.StatusInternalServerError, http.StatusInternalServerError}
	for i, want := range wantStatuses {
		if got := mockStatus(t, http.MethodPost, baseURL+"/v1/deploy", `{"release":"a"}`); got != want {
			t.Fatalf("deploy call %d status = %d, want %d", i+1, got, want)
		}
	}

	// A route the fixture does not declare is a miss, not a 405 or a hang.
	if got := mockStatus(t, http.MethodGet, baseURL+"/not-in-fixture", ""); got != http.StatusNotFound {
		t.Fatalf("unmatched route status = %d, want %d", got, http.StatusNotFound)
	}

	if status := server.Post(baseURL+"/_mock/control/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("mock control exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(15 * time.Second)

	// srd019 AC5: a clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)
}

// TestMockRequestLogRecordsCalls asserts the mock records what a caller sent,
// which is how a validator verifies a subject's outbound calls without parsing
// spans.
//
// Traces srd019-mock AC3 and rel11.0-uc002-mock-serves-fixtures S3.
func TestMockRequestLogRecordsCalls(t *testing.T) {
	RequireCoreRoot(t)
	addr := FreeAddr(t)
	baseURL := "http://" + addr

	server := Serve(t, ServeConfig{
		Profile: filepath.Join("agents", "mock", "profile.yaml"),
		Env: []string{
			"MOCK_ADDRESS=" + addr,
			"MOCK_FIXTURES=" + mockFixture(t, "canned.yaml"),
		},
	})
	defer server.Stop()
	server.WaitHealthy(baseURL+"/_mock/health", 15*time.Second)

	if got := mockStatus(t, http.MethodPost, baseURL+"/v1/deploy", `{"release":"blue"}`); got != http.StatusAccepted {
		t.Fatalf("deploy status = %d, want %d", got, http.StatusAccepted)
	}
	if got := mockStatus(t, http.MethodGet, baseURL+"/v1/status", ""); got != http.StatusOK {
		t.Fatalf("status route = %d, want %d", got, http.StatusOK)
	}
	if got := mockStatus(t, http.MethodGet, baseURL+"/absent", ""); got != http.StatusNotFound {
		t.Fatalf("absent route = %d, want %d", got, http.StatusNotFound)
	}

	entries := mockLog(t, baseURL)
	if len(entries) != 3 {
		t.Fatalf("log has %d entries, want 3: %v", len(entries), entries)
	}

	// Order, method, path, and body are all recoverable, so a validator can
	// assert the subject called the dependency correctly.
	if entries[0]["method"] != http.MethodPost || entries[0]["path"] != "/v1/deploy" {
		t.Fatalf("first entry = %v, want POST /v1/deploy", entries[0])
	}
	if entries[0]["body"] != `{"release":"blue"}` {
		t.Fatalf("first entry body = %v, want the posted body", entries[0]["body"])
	}
	if entries[0]["matched"] != true {
		t.Fatalf("first entry should be matched: %v", entries[0])
	}
	if entries[2]["matched"] != false {
		t.Fatalf("unmatched call should be recorded as a miss: %v", entries[2])
	}

	if status := server.Post(baseURL+"/_mock/control/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("mock control exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(15 * time.Second)
	result.RequireExit(t, 0)
	result.RequireNoErrorSpans(t)
}

// TestMockTwoInstancesDifferentFixtures asserts one shipped profile serves two
// different dependencies at once, differing only by fixture and address, and
// that each instance's log is its own.
//
// Traces srd019-mock AC2, and rel11.0-uc002-mock-serves-fixtures S1 and S5.
func TestMockTwoInstancesDifferentFixtures(t *testing.T) {
	RequireCoreRoot(t)

	type instance struct {
		name    string
		fixture string
		baseURL string
		server  *Server
	}
	instances := []*instance{
		{name: "canned", fixture: "canned.yaml"},
		{name: "scripted", fixture: "scripted.yaml"},
	}

	for _, inst := range instances {
		addr := FreeAddr(t)
		inst.baseURL = "http://" + addr
		inst.server = Serve(t, ServeConfig{
			Profile: filepath.Join("agents", "mock", "profile.yaml"),
			Env: []string{
				"MOCK_ADDRESS=" + addr,
				"MOCK_FIXTURES=" + mockFixture(t, inst.fixture),
			},
		})
		defer inst.server.Stop()
		inst.server.WaitHealthy(inst.baseURL+"/_mock/health", 15*time.Second)
	}

	// The canned instance always accepts; the scripted one fails on its second
	// call. Same profile, different fixture.
	canned, scripted := instances[0], instances[1]
	for i := 0; i < 2; i++ {
		if got := mockStatus(t, http.MethodPost, canned.baseURL+"/v1/deploy", "{}"); got != http.StatusAccepted {
			t.Fatalf("canned deploy call %d = %d, want %d", i+1, got, http.StatusAccepted)
		}
	}
	if got := mockStatus(t, http.MethodPost, scripted.baseURL+"/v1/deploy", "{}"); got != http.StatusOK {
		t.Fatalf("scripted deploy call 1 = %d, want %d", got, http.StatusOK)
	}
	if got := mockStatus(t, http.MethodPost, scripted.baseURL+"/v1/deploy", "{}"); got != http.StatusInternalServerError {
		t.Fatalf("scripted deploy call 2 = %d, want %d", got, http.StatusInternalServerError)
	}

	// Logs are per instance: neither mock sees the other's traffic.
	if got := len(mockLog(t, canned.baseURL)); got != 2 {
		t.Fatalf("canned log has %d entries, want 2", got)
	}
	if got := len(mockLog(t, scripted.baseURL)); got != 2 {
		t.Fatalf("scripted log has %d entries, want 2", got)
	}
}

// TestMockMalformedFixtureFailsStartup asserts a bad fixture stops the mock
// from serving at all, rather than failing on the first request mid-scenario.
//
// Traces srd019-mock AC2 (R2.4) and rel11.0-uc002-mock-serves-fixtures S4.
func TestMockMalformedFixtureFailsStartup(t *testing.T) {
	RequireCoreRoot(t)

	cases := map[string]string{
		"unknown method":  "routes:\n  - method: FETCH\n    path: /x\n    responses:\n      - status: 200\n",
		"empty responses": "routes:\n  - method: GET\n    path: /x\n    responses: []\n",
		"duplicate route": "routes:\n  - method: GET\n    path: /x\n    responses:\n      - status: 200\n  - method: GET\n    path: /x\n    responses:\n      - status: 201\n",
	}

	for name, fixture := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.yaml")
			if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			result := Run(t, RunConfig{
				Profile: filepath.Join("agents", "mock", "profile.yaml"),
				Args:    []string{"--validate-config"},
				Env:     []string{"MOCK_FIXTURES=" + path, "MOCK_ADDRESS=127.0.0.1:0"},
			})
			if result.ExitCode == 0 {
				t.Fatalf("--validate-config accepted a malformed fixture (%s):\n%s", name, result.Output)
			}
		})
	}
}
