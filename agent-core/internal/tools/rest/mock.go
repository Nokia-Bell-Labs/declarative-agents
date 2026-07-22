// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// The mock binding serves canned responses from fixture data so a mock profile
// can stand in for a real upstream service (srd039). Routes are keyed
// by method and literal path, each carrying an ordered response script whose
// last entry repeats once exhausted, and every received request is recorded so
// a validator can assert what the subject under test actually sent.

// mockMethods is the closed set of methods a fixture route may declare.
var mockMethods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true,
	http.MethodHead: true, http.MethodOptions: true,
}

// MockFixture is the on-disk fixture document a mock endpoint serves.
type MockFixture struct {
	Routes []MockRoute `yaml:"routes"`
}

// MockRoute is one method-and-path route with its ordered response script.
type MockRoute struct {
	Method    string         `yaml:"method"`
	Path      string         `yaml:"path"`
	Responses []MockResponse `yaml:"responses"`
}

// MockResponse is one canned response. Body is literal: a string is served
// as-is, any other value is JSON-encoded. The binding never templates or
// computes a body from the request (srd039 R2.5).
type MockResponse struct {
	Status  int               `yaml:"status"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    interface{}       `yaml:"body,omitempty"`
}

// MockConfig configures binding mock. Routes come from a fixture file, from
// inline routes, or both.
type MockConfig struct {
	Fixtures string      `yaml:"fixtures,omitempty"`
	Routes   []MockRoute `yaml:"routes,omitempty"`
}

// MockLogEntry is one recorded request. Matched reports whether a fixture
// route served it; a miss is recorded too (srd039 R2.4).
type MockLogEntry struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`
	Matched bool                `json:"matched"`
}

// mockScript is one route's response script and its position.
type mockScript struct {
	responses []MockResponse
	next      int
}

// mockState holds the merged routes of every mock endpoint on one server and
// the server's request log. The log is per-instance and starts empty, so one
// scenario never observes another's requests (srd039 R3.3).
type mockState struct {
	mu      sync.Mutex
	scripts map[string]*mockScript
	log     []MockLogEntry
}

func mockRouteKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

// newMockState merges the routes of every mock-binding endpoint into one
// state. Fixture files load here, at server start rather than per request
// (srd039 R1.4), so a malformed fixture stops the server from serving.
func newMockState(endpoints map[string]Endpoint) (*mockState, error) {
	names := make([]string, 0, len(endpoints))
	for name := range endpoints {
		names = append(names, name)
	}
	sort.Strings(names)

	state := &mockState{scripts: map[string]*mockScript{}}
	for _, name := range names {
		endpoint := endpoints[name]
		if endpoint.Binding != bindingMock {
			continue
		}
		routes, err := mockRoutes(name, endpoint.Mock)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			key := mockRouteKey(route.Method, route.Path)
			if _, exists := state.scripts[key]; exists {
				return nil, fmt.Errorf("endpoint %q mock fixture declares duplicate route %q", name, key)
			}
			state.scripts[key] = &mockScript{responses: route.Responses}
		}
	}
	return state, nil
}

// mockRoutes reads and validates one endpoint's routes from its fixture file
// and inline config.
func mockRoutes(name string, cfg *MockConfig) ([]MockRoute, error) {
	if cfg == nil {
		return nil, fmt.Errorf("endpoint %q binding %s requires mock config with fixtures or routes", name, bindingMock)
	}
	routes := append([]MockRoute{}, cfg.Routes...)
	if cfg.Fixtures != "" {
		fileRoutes, err := loadMockFixture(cfg.Fixtures)
		if err != nil {
			return nil, fmt.Errorf("endpoint %q mock fixture: %w", name, err)
		}
		routes = append(routes, fileRoutes...)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("endpoint %q mock config declares no routes", name)
	}
	if err := validateMockRoutes(name, routes); err != nil {
		return nil, err
	}
	return routes, nil
}

// loadMockFixture reads a fixture file, applying the same environment
// expansion as other REST definitions (srd039 R1.3).
func loadMockFixture(path string) ([]MockRoute, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var fixture MockFixture
	if err := yaml.Unmarshal(expandEnv(data), &fixture); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return fixture.Routes, nil
}

// validateMockRoutes rejects an unknown method, an empty response list, a
// missing path, and a duplicate method-and-path route, each with a named
// error (srd039 R4.1).
func validateMockRoutes(name string, routes []MockRoute) error {
	seen := map[string]bool{}
	for i, route := range routes {
		method := strings.ToUpper(route.Method)
		if !mockMethods[method] {
			return fmt.Errorf("endpoint %q mock route %d declares unknown method %q", name, i, route.Method)
		}
		if !strings.HasPrefix(route.Path, "/") {
			return fmt.Errorf("endpoint %q mock route %d declares path %q; want a path beginning with /", name, i, route.Path)
		}
		if len(route.Responses) == 0 {
			return fmt.Errorf("endpoint %q mock route %s %s declares no responses", name, method, route.Path)
		}
		for j, response := range route.Responses {
			if response.Status < 100 || response.Status > 599 {
				return fmt.Errorf("endpoint %q mock route %s %s response %d declares status %d; want 100-599",
					name, method, route.Path, j, response.Status)
			}
		}
		key := mockRouteKey(method, route.Path)
		if seen[key] {
			return fmt.Errorf("endpoint %q mock config declares duplicate route %q", name, key)
		}
		seen[key] = true
	}
	return nil
}

// next returns the response for this call and advances the script. Once the
// script is exhausted the last response repeats (srd039 R2.3). Returns false
// when no route matches.
func (m *mockState) next(method, path string) (MockResponse, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	script, ok := m.scripts[mockRouteKey(method, path)]
	if !ok {
		return MockResponse{}, false
	}
	index := script.next
	if index >= len(script.responses) {
		index = len(script.responses) - 1
	} else {
		script.next++
	}
	return script.responses[index], true
}

func (m *mockState) record(entry MockLogEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.log = append(m.log, entry)
}

func (m *mockState) entries() []MockLogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MockLogEntry, len(m.log))
	copy(out, m.log)
	return out
}

// serveMock answers one request from the fixture script and records it. It
// reads the request directly rather than through declared-parameter
// validation, because a mock serves whatever methods and paths its fixture
// declares rather than one declared route shape.
func (r *serverRuntime) serveMock(w http.ResponseWriter, req *http.Request) {
	body := readMockBody(req, r.def.Limits.MaxRequestBytes)
	response, matched := r.mock.next(req.Method, req.URL.Path)
	r.mock.record(MockLogEntry{
		Method:  req.Method,
		Path:    req.URL.Path,
		Headers: req.Header.Clone(),
		Body:    body,
		Matched: matched,
	})
	if !matched {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "no mock route for " + mockRouteKey(req.Method, req.URL.Path),
		})
		return
	}
	writeMockResponse(w, response)
}

// writeMockLog serves the recorded requests so a validator can assert what
// the subject sent (srd039 R3.2).
func (r *serverRuntime) writeMockLog(w http.ResponseWriter) {
	entries := r.mock.entries()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"count":    len(entries),
		"requests": entries,
	})
}

func writeMockResponse(w http.ResponseWriter, response MockResponse) {
	payload, isJSON := mockBody(response.Body)
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}
	if w.Header().Get("Content-Type") == "" && isJSON {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(response.Status)
	if len(payload) > 0 {
		_, _ = w.Write(payload)
	}
}

// mockBody renders a literal body. A string is served as-is; any other value
// is JSON-encoded so fixtures can declare structured responses in YAML.
func mockBody(body interface{}) ([]byte, bool) {
	switch value := body.(type) {
	case nil:
		return nil, false
	case string:
		return []byte(value), false
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		return encoded, true
	}
}

func readMockBody(req *http.Request, limit int) string {
	if req.Body == nil {
		return ""
	}
	var reader io.Reader = req.Body
	if limit > 0 {
		reader = io.LimitReader(req.Body, int64(limit))
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return ""
	}
	return string(data)
}
