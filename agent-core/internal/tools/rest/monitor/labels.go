// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package monitor

import "strings"

// Duplicated from rest/redaction.go until GH-1822 extracts the redaction
// cluster. Importing rest from here would cycle with the parent's monitor import.
var sensitiveRedactionTerms = []string{
	"prompt", "secret", "token", "authorization", "full_output",
	"request_id", "timestamp", "stack_trace", "command_output", "url",
	"path", "user_text",
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

func safeLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for name, value := range labels {
		if safeRedactionLabel(name, value) {
			out[name] = value
		}
	}
	return out
}
