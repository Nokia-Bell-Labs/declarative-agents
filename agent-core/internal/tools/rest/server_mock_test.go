// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockServer builds a mock-profile-shaped server: a catch-all mock mount, a log route,
// and health. The mock mount answers whatever methods and paths its fixture
// declares, so the literal routes must stay more specific than the catch-all.
func mockServer(t *testing.T, name string, cfg *MockConfig) Server {
	t.Helper()
	return Server{
		Address: "127.0.0.1:0",
		Queue:   QueueConfig{Name: name, Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]Endpoint{
			"mock": {
				Method: http.MethodGet, Path: "/{path...}", Binding: bindingMock, Mock: cfg,
				Request: RequestBinding{Path: map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				}},
			},
			"log":    {Method: http.MethodGet, Path: "/_mock/log", Binding: bindingMockLog},
			"health": {Method: http.MethodGet, Path: "/_mock/health", Binding: bindingHealth},
		},
	}
}

func writeFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func readMockLog(t *testing.T, baseURL string) []map[string]interface{} {
	t.Helper()
	raw := requestBody(t, http.MethodGet, baseURL+"/_mock/log", "", http.StatusOK)
	var payload struct {
		Count    int                      `json:"count"`
		Requests []map[string]interface{} `json:"requests"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	require.Equal(t, payload.Count, len(payload.Requests))
	return payload.Requests
}

// TestRESTServer_MockBindingServesScriptedResponses covers srd039 AC1 and AC2:
// a route's responses are served in declared order, the last repeats once the
// script is exhausted, and an unmatched route gets a 404.
func TestRESTServer_MockBindingServesScriptedResponses(t *testing.T) {
	t.Parallel()

	fixture := writeFixture(t, `
routes:
  - method: POST
    path: /deploy
    responses:
      - status: 200
        body: {ok: true}
      - status: 500
        body: "boom"
`)
	state, baseURL := launchRESTServer(t, mockServer(t, "mock_seq", &MockConfig{Fixtures: fixture}), LimitProfile{})
	defer stopRESTServer(t, state, "mock_seq")

	first := requestBody(t, http.MethodPost, baseURL+"/deploy", "", http.StatusOK)
	require.JSONEq(t, `{"ok":true}`, first)

	// The script is exhausted after the second call, so the last response repeats.
	for i := 0; i < 2; i++ {
		body := requestBody(t, http.MethodPost, baseURL+"/deploy", "", http.StatusInternalServerError)
		require.Equal(t, "boom", body, "call %d", i+2)
	}

	requestBody(t, http.MethodGet, baseURL+"/nope", "", http.StatusNotFound)
	// A declared route answers only its declared method; another method is a miss.
	requestBody(t, http.MethodGet, baseURL+"/deploy", "", http.StatusNotFound)
}

// TestRESTServer_MockBindingRequestLog covers srd039 AC3: the log returns the
// received requests in order with method, path, headers, and body, and marks
// an unmatched request as a miss.
func TestRESTServer_MockBindingRequestLog(t *testing.T) {
	t.Parallel()

	fixture := writeFixture(t, `
routes:
  - method: POST
    path: /deploy
    responses:
      - status: 202
        body: "accepted"
`)
	state, baseURL := launchRESTServer(t, mockServer(t, "mock_log", &MockConfig{Fixtures: fixture}), LimitProfile{})
	defer stopRESTServer(t, state, "mock_log")

	requestBody(t, http.MethodPost, baseURL+"/deploy", `{"release":"a"}`, http.StatusAccepted)
	requestBody(t, http.MethodGet, baseURL+"/nope", "", http.StatusNotFound)

	entries := readMockLog(t, baseURL)
	require.Len(t, entries, 2)

	require.Equal(t, http.MethodPost, entries[0]["method"])
	require.Equal(t, "/deploy", entries[0]["path"])
	require.Equal(t, `{"release":"a"}`, entries[0]["body"])
	require.Equal(t, true, entries[0]["matched"])
	headers, ok := entries[0]["headers"].(map[string]interface{})
	require.True(t, ok, "headers should be recorded")
	require.Contains(t, headers, "Content-Type")

	require.Equal(t, http.MethodGet, entries[1]["method"])
	require.Equal(t, "/nope", entries[1]["path"])
	require.Equal(t, false, entries[1]["matched"], "an unmatched request is recorded as a miss")
}

// TestRESTServer_MockBindingFixtureValidation covers srd039 AC4: a malformed
// fixture fails validation and prevents the server from serving, with a named
// error, rather than failing on the first request.
func TestRESTServer_MockBindingFixtureValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		fixture string
		want    string
	}{
		{
			name:    "unknown method",
			fixture: "routes:\n  - method: FETCH\n    path: /x\n    responses:\n      - status: 200\n",
			want:    "unknown method",
		},
		{
			name:    "empty responses",
			fixture: "routes:\n  - method: GET\n    path: /x\n    responses: []\n",
			want:    "declares no responses",
		},
		{
			name: "duplicate route",
			fixture: "routes:\n  - method: GET\n    path: /x\n    responses:\n      - status: 200\n" +
				"  - method: GET\n    path: /x\n    responses:\n      - status: 201\n",
			want: "duplicate route",
		},
		{
			name:    "status out of range",
			fixture: "routes:\n  - method: GET\n    path: /x\n    responses:\n      - status: 99\n",
			want:    "want 100-599",
		},
		{
			name:    "path without leading slash",
			fixture: "routes:\n  - method: GET\n    path: x\n    responses:\n      - status: 200\n",
			want:    "want a path beginning with /",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			endpoint := Endpoint{
				Method: http.MethodGet, Path: "/{path...}", Binding: bindingMock,
				Mock: &MockConfig{Fixtures: writeFixture(t, tc.fixture)},
			}

			// --validate-config path.
			err := validateEndpoint("mock", endpoint)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)

			// Server startup path: the listener must never come up.
			_, err = newServerRuntime(ServerDefinition{
				Name:   "mock_invalid",
				Server: Server{Address: "127.0.0.1:0", Endpoints: map[string]Endpoint{"mock": endpoint}},
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestRESTServer_MockBindingConcurrentSequencing covers srd039 AC5: concurrent
// requests consume each script position exactly once, and every request is
// recorded.
func TestRESTServer_MockBindingConcurrentSequencing(t *testing.T) {
	t.Parallel()

	const calls = 8
	fixture := "routes:\n  - method: GET\n    path: /seq\n    responses:\n"
	for i := 0; i < calls; i++ {
		fixture += fmt.Sprintf("      - status: 200\n        body: \"%d\"\n", i)
	}
	state, baseURL := launchRESTServer(t, mockServer(t, "mock_conc", &MockConfig{Fixtures: writeFixture(t, fixture)}), LimitProfile{})
	defer stopRESTServer(t, state, "mock_conc")

	// A dedicated client without keep-alives: the shared DefaultClient pool would
	// leave connections lingering after a burst, and Shutdown waits on them.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	defer client.CloseIdleConnections()

	var wg sync.WaitGroup
	bodies := make([]string, calls)
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			resp, err := client.Get(baseURL + "/seq")
			if err != nil {
				t.Errorf("call %d: %v", slot, err)
				return
			}
			defer func() { _ = resp.Body.Close() }()
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("call %d read: %v", slot, err)
				return
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("call %d: status %d", slot, resp.StatusCode)
			}
			bodies[slot] = string(data)
		}(i)
	}
	wg.Wait()

	seen := map[string]int{}
	for _, body := range bodies {
		seen[body]++
	}
	require.Len(t, seen, calls, "each script position should be served exactly once: %v", seen)
	for position, count := range seen {
		require.Equal(t, 1, count, "position %s served %d times", position, count)
	}
	require.Len(t, readMockLog(t, baseURL), calls)
}

// TestRESTServer_MockBindingInlineRoutesAndEnv covers srd039 R1.3: routes may
// be declared inline, and fixture content is environment-expanded like other
// REST definitions.
func TestRESTServer_MockBindingInlineRoutesAndEnv(t *testing.T) {
	t.Setenv("MOCK_TEST_BODY", "from-env")

	fixture := writeFixture(t, `
routes:
  - method: GET
    path: /env
    responses:
      - status: 200
        body: "${MOCK_TEST_BODY:-fallback}"
`)
	server := mockServer(t, "mock_inline", &MockConfig{
		Fixtures: fixture,
		Routes: []MockRoute{{
			Method: http.MethodGet, Path: "/inline",
			Responses: []MockResponse{{Status: 200, Body: "inline-body"}},
		}},
	})
	state, baseURL := launchRESTServer(t, server, LimitProfile{})
	defer stopRESTServer(t, state, "mock_inline")

	require.Equal(t, "inline-body", requestBody(t, http.MethodGet, baseURL+"/inline", "", http.StatusOK))
	require.Equal(t, "from-env", requestBody(t, http.MethodGet, baseURL+"/env", "", http.StatusOK))
}

// TestRESTServer_MockBindingLogIsPerInstance covers srd039 R3.3: a fresh server
// starts with an empty log.
func TestRESTServer_MockBindingLogIsPerInstance(t *testing.T) {
	t.Parallel()

	cfg := &MockConfig{Routes: []MockRoute{{
		Method: http.MethodGet, Path: "/ping",
		Responses: []MockResponse{{Status: 200, Body: "pong"}},
	}}}

	state, baseURL := launchRESTServer(t, mockServer(t, "mock_inst_a", cfg), LimitProfile{})
	requestBody(t, http.MethodGet, baseURL+"/ping", "", http.StatusOK)
	require.Len(t, readMockLog(t, baseURL), 1)
	stopRESTServer(t, state, "mock_inst_a")

	second, secondURL := launchRESTServer(t, mockServer(t, "mock_inst_b", cfg), LimitProfile{})
	defer stopRESTServer(t, second, "mock_inst_b")
	require.Empty(t, readMockLog(t, secondURL), "a fresh instance starts with an empty log")
}

// TestRESTServer_MockConfigBindingMismatch covers the config-shape guard: mock
// config on another binding, and a mock binding with no routes, are rejected.
func TestRESTServer_MockConfigBindingMismatch(t *testing.T) {
	t.Parallel()

	err := validateEndpoint("m", Endpoint{
		Method: http.MethodGet, Path: "/x", Binding: bindingHealth,
		Mock: &MockConfig{Routes: []MockRoute{{Method: "GET", Path: "/x", Responses: []MockResponse{{Status: 200}}}}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "has mock config but binding is")

	err = validateEndpoint("m", Endpoint{Method: http.MethodGet, Path: "/x", Binding: bindingMock})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires mock config")

	err = validateEndpoint("m", Endpoint{
		Method: http.MethodGet, Path: "/x", Binding: bindingMock, Mock: &MockConfig{},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "declares no routes")
}
