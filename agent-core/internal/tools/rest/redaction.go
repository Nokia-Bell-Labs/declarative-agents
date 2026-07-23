// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	redactionBody    = "body"
	redactionHeaders = "headers"
	redactionQuery   = "query"
	redactedValue    = "[REDACTED]"
)

var sensitiveRedactionTerms = []string{
	"prompt", "secret", "token", "authorization", "full_output",
	"request_id", "timestamp", "stack_trace", "command_output", "url",
	"path", "user_text",
}

func clientRedactionSelectors(def ClientOperationDefinition, mapping StatusMapping) []string {
	responseMap := resolvedResponseMapping(def, mapping)
	selectors := append([]string{}, responseMap.Redact...)
	selectors = append(selectors, authRedactionSelectors(def.Auth)...)
	return selectors
}

// clientOutputRedaction converts REST selector syntax into typed paths through
// the emitted Result output. REST keeps its immediate marker-based response
// contract; core uses these paths to remove marked fields from Execution.
func clientOutputRedaction(
	def ClientOperationDefinition,
	mapping StatusMapping,
	selectors []string,
) core.OutputRedaction {
	responseMap := resolvedResponseMapping(def, mapping)
	var paths []core.OutputRedactionPath
	for _, selector := range selectors {
		scope, field, ok := parseRedactionSelector(selector)
		if !ok {
			continue
		}
		switch scope {
		case redactionBody:
			paths = appendUniqueOutputPath(paths, append(
				core.OutputRedactionPath{"body"},
				strings.Split(field, ".")...,
			))
			for name, source := range responseMap.Output {
				if sameBodyRedactionField(source, field) {
					paths = appendUniqueOutputPath(paths, core.OutputRedactionPath{"mapped", name})
				}
			}
			if sameBodyRedactionField(responseMap.ResourceID, field) {
				paths = appendUniqueOutputPath(paths, core.OutputRedactionPath{"resource_id"})
			}
			if sameBodyRedactionField(responseMap.RequestID, field) {
				paths = appendUniqueOutputPath(paths, core.OutputRedactionPath{"request_id"})
			}
		case redactionHeaders:
			paths = appendUniqueOutputPath(paths, core.OutputRedactionPath{
				"headers",
				strings.ToLower(field),
			})
		}
	}
	return core.OutputRedaction{Version: core.OutputRedactionVersion1, Paths: paths}
}

func sameBodyRedactionField(selector, field string) bool {
	scope, selectedField, ok := parseRedactionSelector(selector)
	return ok && scope == redactionBody && selectedField == field
}

func appendUniqueOutputPath(
	paths []core.OutputRedactionPath,
	candidate core.OutputRedactionPath,
) []core.OutputRedactionPath {
	for _, path := range paths {
		if strings.Join(path, "\x00") == strings.Join(candidate, "\x00") {
			return paths
		}
	}
	return append(paths, candidate)
}

func authRedactionSelectors(auth AuthProfile) []string {
	var selectors []string
	if auth.Header != "" {
		selectors = append(selectors, "headers."+strings.ToLower(auth.Header))
	}
	if auth.Query != "" {
		selectors = append(selectors, "query."+auth.Query)
	}
	return selectors
}

func redactClientOutput(output map[string]interface{}, selectors []string) {
	for _, selector := range selectors {
		applyRedactionSelector(selector, func(scope, field string) {
			switch scope {
			case redactionBody:
				redactNested(output["body"], field)
			case redactionHeaders:
				redactNested(output["headers"], field)
			}
		})
	}
}

func redactClientDerivedOutput(
	output map[string]interface{},
	responseMap ResponseMapping,
	selectors []string,
) {
	mapped, _ := output["mapped"].(map[string]interface{})
	for _, selector := range selectors {
		scope, field, ok := parseRedactionSelector(selector)
		if !ok || scope != redactionBody {
			continue
		}
		for name, source := range responseMap.Output {
			if sameBodyRedactionField(source, field) {
				mapped[name] = redactedValue
			}
		}
		if sameBodyRedactionField(responseMap.ResourceID, field) {
			output["resource_id"] = redactedValue
		}
		if sameBodyRedactionField(responseMap.RequestID, field) {
			output["request_id"] = redactedValue
		}
	}
}

func redactServerPayload(payload map[string]interface{}, selectors []string) {
	for _, selector := range selectors {
		applyRedactionSelector(selector, func(scope, field string) {
			redactServerField(payload, scope, field)
		})
	}
}

func redactServerField(payload map[string]interface{}, scope, field string) {
	switch scope {
	case redactionBody:
		redactNested(payload["body"], field)
	case redactionHeaders:
		redactNested(payload["headers"], field)
	case redactionQuery:
		redactNested(payload["query"], field)
	}
	if field != "" {
		payload[field] = redactedValue
	}
}

func redactMappedOutput(output map[string]interface{}, selectors []string) {
	for _, selector := range selectors {
		applyRedactionSelector(selector, func(_ string, field string) {
			output[field] = redactedValue
		})
	}
}

func redactTextValues(text string, values []string) string {
	for _, value := range values {
		if value != "" {
			text = strings.ReplaceAll(text, value, redactedValue)
		}
	}
	return text
}

func safeRedactionLabel(name, value string) bool {
	if name == "" || value == "" {
		return false
	}
	combined := strings.ToLower(name + " " + value)
	for _, term := range sensitiveRedactionTerms {
		if strings.Contains(combined, term) {
			return false
		}
	}
	return true
}

func validRedactionSelector(selector string) bool {
	_, _, ok := parseRedactionSelector(selector)
	return ok
}

func applyRedactionSelector(selector string, apply func(scope, field string)) {
	scope, field, ok := parseRedactionSelector(selector)
	if !ok {
		return
	}
	apply(scope, field)
}

func parseRedactionSelector(selector string) (string, string, bool) {
	switch {
	case strings.HasPrefix(selector, "$."):
		return redactionBody, strings.TrimPrefix(selector, "$."), len(selector) > 2
	case strings.HasPrefix(selector, "body."):
		return redactionBody, strings.TrimPrefix(selector, "body."), len(selector) > len("body.")
	case strings.HasPrefix(selector, "headers."):
		return redactionHeaders, strings.TrimPrefix(selector, "headers."), len(selector) > len("headers.")
	case strings.HasPrefix(selector, "query."):
		return redactionQuery, strings.TrimPrefix(selector, "query."), len(selector) > len("query.")
	default:
		return "", "", false
	}
}

func redactNested(value interface{}, field string) {
	values, ok := value.(map[string]interface{})
	if !ok || field == "" {
		return
	}
	values[strings.ToLower(field)] = redactedValue
	values[field] = redactedValue
}
