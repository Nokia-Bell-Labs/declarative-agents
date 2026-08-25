// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

// Mock types are declaration schema: they belong with the REST model, not the
// mock engine. The engine in mock.go consumes these types and never the reverse.

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
