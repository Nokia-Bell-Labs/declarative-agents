// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "strings"

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
