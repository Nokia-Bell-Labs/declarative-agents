// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// The mock binding serves canned responses from fixture data so a mock profile
// can stand in for a real upstream service (srd039). Routes are keyed by method
// and literal path, each carrying an ordered response script whose last entry
// repeats once exhausted, and every received request is recorded so a validator
// can assert what the subject under test actually sent.

var mockMethods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true,
	http.MethodHead: true, http.MethodOptions: true,
}

type script struct {
	responses []Response
	next      int
}

// Engine holds the merged routes of every mock endpoint on one server and the
// server's request log. The log is per-instance and starts empty, so one
// scenario never observes another's requests (srd039 R3.3).
type Engine struct {
	mu      sync.Mutex
	scripts map[string]*script
	log     []LogEntry
}

func routeKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

// New merges the routes of every mock-binding endpoint into one engine.
// Fixture files load here, at server start rather than per request (srd039 R1.4).
func New(endpoints []EndpointSpec) (*Engine, error) {
	names := make([]string, 0, len(endpoints))
	byName := map[string]EndpointSpec{}
	for _, spec := range endpoints {
		names = append(names, spec.Name)
		byName[spec.Name] = spec
	}
	sort.Strings(names)
	engine := &Engine{scripts: map[string]*script{}}
	for _, name := range names {
		if err := engine.addEndpoint(byName[name]); err != nil {
			return nil, err
		}
	}
	return engine, nil
}

func (e *Engine) addEndpoint(spec EndpointSpec) error {
	routes, err := LoadRoutes(spec.Name, spec.Config)
	if err != nil {
		return err
	}
	for _, route := range routes {
		key := routeKey(route.Method, route.Path)
		if _, exists := e.scripts[key]; exists {
			return fmt.Errorf("endpoint %q mock fixture declares duplicate route %q", spec.Name, key)
		}
		e.scripts[key] = &script{responses: route.Responses}
	}
	return nil
}

func (e *Engine) next(method, path string) (Response, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	script, ok := e.scripts[routeKey(method, path)]
	if !ok {
		return Response{}, false
	}
	index := script.next
	if index >= len(script.responses) {
		index = len(script.responses) - 1
	} else {
		script.next++
	}
	return script.responses[index], true
}

func (e *Engine) record(entry LogEntry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.log = append(e.log, entry)
}

func (e *Engine) entries() []LogEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]LogEntry, len(e.log))
	copy(out, e.log)
	return out
}

// Serve answers one request from the fixture script and records it.
func (e *Engine) Serve(w http.ResponseWriter, req *http.Request, maxRequestBytes int) {
	body := readBody(req, maxRequestBytes)
	response, matched := e.next(req.Method, req.URL.Path)
	e.record(LogEntry{
		Method: req.Method, Path: req.URL.Path,
		Headers: req.Header.Clone(), Body: body, Matched: matched,
	})
	if !matched {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "no mock route for " + routeKey(req.Method, req.URL.Path),
		})
		return
	}
	writeResponse(w, response)
}

// WriteLog serves the recorded requests so a validator can assert what the
// subject sent (srd039 R3.2).
func (e *Engine) WriteLog(w http.ResponseWriter) {
	entries := e.entries()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(entries), "requests": entries,
	})
}

func writeResponse(w http.ResponseWriter, response Response) {
	payload, isJSON := encodeBody(response.Body)
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

func encodeBody(body interface{}) ([]byte, bool) {
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

func readBody(req *http.Request, limit int) string {
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

func writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
