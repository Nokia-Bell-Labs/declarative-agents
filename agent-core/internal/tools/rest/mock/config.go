// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package mock

// Config, Route, and Response are the engine's fixture model. The parent
// converts rest.MockConfig at the package boundary so this package does not
// import rest (srd039).

// Fixture is the on-disk fixture document a mock endpoint serves.
type Fixture struct {
	Routes []Route `yaml:"routes"`
}

// Route is one method-and-path route with its ordered response script.
type Route struct {
	Method    string     `yaml:"method"`
	Path      string     `yaml:"path"`
	Responses []Response `yaml:"responses"`
}

// Response is one canned response. Body is literal: a string is served as-is,
// any other value is JSON-encoded.
type Response struct {
	Status  int               `yaml:"status"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    interface{}       `yaml:"body,omitempty"`
}

// Config configures binding mock. Routes come from a fixture file, from
// inline routes, or both.
type Config struct {
	Fixtures string  `yaml:"fixtures,omitempty"`
	Routes   []Route `yaml:"routes,omitempty"`
}

// LogEntry is one recorded request. Matched reports whether a fixture route
// served it; a miss is recorded too (srd039 R2.4).
type LogEntry struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`
	Matched bool                `json:"matched"`
}

// EndpointSpec is one mock-binding endpoint's name and config.
type EndpointSpec struct {
	Name   string
	Config *Config
}
