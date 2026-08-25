// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package redact applies REST redaction selectors (srd028, srd038).
// It does not import rest or client; callers convert AuthProfile/ResponseMapping
// at the package boundary. monitor cannot import this package (GH-1822 DAG).
package redact

import (
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	scopeBody    = "body"
	scopeHeaders = "headers"
	scopeQuery   = "query"
	redacted     = "[REDACTED]"
)

// Value is the marker written over redacted fields.
const Value = redacted

var sensitiveTerms = []string{
	"prompt", "secret", "token", "authorization", "full_output",
	"request_id", "timestamp", "stack_trace", "command_output", "url",
	"path", "user_text",
}

// Auth is the header/query names a credential profile may leak.
type Auth struct {
	Header string
	Query  string
}

// Mapping is the response-mapping fields redaction reads.
type Mapping struct {
	Schema     map[string]interface{}
	Output     map[string]string
	Redact     []string
	ResourceID string
	RequestID  string
}

// Status is the status-mapping fields used to resolve a Mapping.
type Status struct {
	ResponseRef string
	Response    Mapping
}

// Source is the operation-level mapping data resolvedResponseMapping needs.
type Source struct {
	Auth             Auth
	ResponseMappings map[string]Mapping
	Operation        Mapping
	OperationRef     string
}

// ResolveMapping picks the response mapping for one status mapping.
func ResolveMapping(src Source, status Status) Mapping {
	if status.ResponseRef != "" {
		return src.ResponseMappings[status.ResponseRef]
	}
	if !emptyMapping(status.Response) {
		return status.Response
	}
	if src.OperationRef != "" {
		return src.ResponseMappings[src.OperationRef]
	}
	return src.Operation
}

func emptyMapping(mapping Mapping) bool {
	return len(mapping.Schema) == 0 && len(mapping.Output) == 0 && len(mapping.Redact) == 0 &&
		mapping.ResourceID == "" && mapping.RequestID == ""
}

// Selectors is the client redaction selector list for one status mapping.
func Selectors(src Source, status Status) []string {
	responseMap := ResolveMapping(src, status)
	selectors := append([]string{}, responseMap.Redact...)
	selectors = append(selectors, authSelectors(src.Auth)...)
	return selectors
}

func authSelectors(auth Auth) []string {
	var selectors []string
	if auth.Header != "" {
		selectors = append(selectors, "headers."+strings.ToLower(auth.Header))
	}
	if auth.Query != "" {
		selectors = append(selectors, "query."+auth.Query)
	}
	return selectors
}

// OutputRedaction converts REST selector syntax into typed Result paths.
func OutputRedaction(src Source, status Status, selectors []string) core.OutputRedaction {
	responseMap := ResolveMapping(src, status)
	var paths []core.OutputRedactionPath
	for _, selector := range selectors {
		scope, field, ok := parseSelector(selector)
		if !ok {
			continue
		}
		switch scope {
		case scopeBody:
			paths = appendBodyPaths(paths, responseMap, field)
		case scopeHeaders:
			paths = appendUnique(paths, core.OutputRedactionPath{"headers", strings.ToLower(field)})
		}
	}
	return core.OutputRedaction{Version: core.OutputRedactionVersion1, Paths: paths}
}

func appendBodyPaths(paths []core.OutputRedactionPath, responseMap Mapping, field string) []core.OutputRedactionPath {
	paths = appendUnique(paths, append(core.OutputRedactionPath{"body"}, strings.Split(field, ".")...))
	for name, source := range responseMap.Output {
		if sameBodyField(source, field) {
			paths = appendUnique(paths, core.OutputRedactionPath{"mapped", name})
		}
	}
	if sameBodyField(responseMap.ResourceID, field) {
		paths = appendUnique(paths, core.OutputRedactionPath{"resource_id"})
	}
	if sameBodyField(responseMap.RequestID, field) {
		paths = appendUnique(paths, core.OutputRedactionPath{"request_id"})
	}
	return paths
}

func sameBodyField(selector, field string) bool {
	scope, selected, ok := parseSelector(selector)
	return ok && scope == scopeBody && selected == field
}

func appendUnique(paths []core.OutputRedactionPath, candidate core.OutputRedactionPath) []core.OutputRedactionPath {
	for _, path := range paths {
		if strings.Join(path, "\x00") == strings.Join(candidate, "\x00") {
			return paths
		}
	}
	return append(paths, candidate)
}

// ClientOutput redacts body and header fields on a client Result output map.
func ClientOutput(output map[string]interface{}, selectors []string) {
	for _, selector := range selectors {
		applySelector(selector, func(scope, field string) {
			switch scope {
			case scopeBody:
				redactNested(output["body"], field)
			case scopeHeaders:
				redactNested(output["headers"], field)
			}
		})
	}
}

// DerivedOutput redacts mapped resource_id/request_id fields.
func DerivedOutput(output map[string]interface{}, mapping Mapping, selectors []string) {
	mapped, _ := output["mapped"].(map[string]interface{})
	for _, selector := range selectors {
		scope, field, ok := parseSelector(selector)
		if !ok || scope != scopeBody {
			continue
		}
		for name, source := range mapping.Output {
			if sameBodyField(source, field) {
				mapped[name] = redacted
			}
		}
		if sameBodyField(mapping.ResourceID, field) {
			output["resource_id"] = redacted
		}
		if sameBodyField(mapping.RequestID, field) {
			output["request_id"] = redacted
		}
	}
}

// ServerPayload redacts a server request payload in place.
func ServerPayload(payload map[string]interface{}, selectors []string) {
	for _, selector := range selectors {
		applySelector(selector, func(scope, field string) {
			redactServerField(payload, scope, field)
		})
	}
}

func redactServerField(payload map[string]interface{}, scope, field string) {
	switch scope {
	case scopeBody:
		redactNested(payload["body"], field)
	case scopeHeaders:
		redactNested(payload["headers"], field)
	case scopeQuery:
		redactNested(payload["query"], field)
	}
	if field != "" {
		payload[field] = redacted
	}
}

// MappedOutput redacts mapped output fields by selector field name.
func MappedOutput(output map[string]interface{}, selectors []string) {
	for _, selector := range selectors {
		applySelector(selector, func(_ string, field string) {
			output[field] = redacted
		})
	}
}

// TextValues replaces secret substrings with the redaction marker.
func TextValues(text string, values []string) string {
	for _, value := range values {
		if value != "" {
			text = strings.ReplaceAll(text, value, redacted)
		}
	}
	return text
}

// SafeLabel reports whether a metric label name/value pair may be emitted.
func SafeLabel(name, value string) bool {
	if name == "" || value == "" {
		return false
	}
	combined := strings.ToLower(name + " " + value)
	for _, term := range sensitiveTerms {
		if strings.Contains(combined, term) {
			return false
		}
	}
	return true
}

// SafeLabels keeps only labels SafeLabel accepts.
func SafeLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for name, value := range labels {
		if SafeLabel(name, value) {
			out[name] = value
		}
	}
	return out
}

// ValidSelector reports whether a redaction selector is well-formed.
func ValidSelector(selector string) bool {
	_, _, ok := parseSelector(selector)
	return ok
}

func applySelector(selector string, apply func(scope, field string)) {
	scope, field, ok := parseSelector(selector)
	if !ok {
		return
	}
	apply(scope, field)
}

func parseSelector(selector string) (string, string, bool) {
	switch {
	case strings.HasPrefix(selector, "$."):
		return scopeBody, strings.TrimPrefix(selector, "$."), len(selector) > 2
	case strings.HasPrefix(selector, "body."):
		return scopeBody, strings.TrimPrefix(selector, "body."), len(selector) > len("body.")
	case strings.HasPrefix(selector, "headers."):
		return scopeHeaders, strings.TrimPrefix(selector, "headers."), len(selector) > len("headers.")
	case strings.HasPrefix(selector, "query."):
		return scopeQuery, strings.TrimPrefix(selector, "query."), len(selector) > len("query.")
	default:
		return "", "", false
	}
}

func redactNested(value interface{}, field string) {
	values, ok := value.(map[string]interface{})
	if !ok || field == "" {
		return
	}
	values[strings.ToLower(field)] = redacted
	values[field] = redacted
}
