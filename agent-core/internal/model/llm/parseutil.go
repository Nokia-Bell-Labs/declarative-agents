// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FixNewlinesInStrings replaces literal newlines and tabs inside JSON
// string values with their escape sequences. Some models output actual
// newline bytes inside strings instead of the required \n escape.
func FixNewlinesInStrings(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			buf.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && inString {
			buf.WriteByte(c)
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			buf.WriteByte(c)
			continue
		}
		if inString {
			switch c {
			case '\n':
				buf.WriteString(`\n`)
			case '\r':
				buf.WriteString(`\r`)
			case '\t':
				buf.WriteString(`\t`)
			default:
				buf.WriteByte(c)
			}
		} else {
			buf.WriteByte(c)
		}
	}
	return buf.String()
}

// ExtractFlatParams handles models that put parameters at the top level
// alongside "tool" instead of nesting them under a "parameters" key.
// E.g. {"tool":"edit","path":"f.go"} becomes {"path":"f.go"}.
func ExtractFlatParams(raw, tool string) json.RawMessage {
	var flat map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &flat); err != nil {
		return json.RawMessage(`{}`)
	}
	delete(flat, "tool")
	if len(flat) == 0 {
		return json.RawMessage(`{}`)
	}
	out, err := json.Marshal(flat)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return out
}

// CountToolCallBlocks counts how many [tool_call] blocks appear in raw
// output. Used to detect when a model sends multiple tool calls in one
// response.
func CountToolCallBlocks(raw string) int {
	return strings.Count(raw, "[tool_call]")
}

// EstimateTokens provides a rough token count by summing content
// lengths and dividing by 4.
func EstimateTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	return total / 4
}

// ExtractDoneSummary pulls the "summary" string from done-tool
// parameters. Falls back to the raw JSON if the field is absent.
func ExtractDoneSummary(params json.RawMessage) string {
	var p struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Summary == "" {
		return string(params)
	}
	return p.Summary
}

// CheckRequiredFields extracts the "required" array from a JSON Schema
// and verifies each listed key is present in params. Returns the names
// of missing fields.
func CheckRequiredFields(schema json.RawMessage, params json.RawMessage) []string {
	if len(schema) == 0 {
		return nil
	}

	var s struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &s); err != nil || len(s.Required) == 0 {
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(params, &fields); err != nil {
		return s.Required
	}

	var missing []string
	for _, name := range s.Required {
		if _, ok := fields[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

// ClassifyParseError returns a short category for the error text.
func ClassifyParseError(errText string) string {
	lower := strings.ToLower(errText)
	switch {
	case strings.Contains(lower, "malformed json"):
		return "malformed_json"
	case strings.Contains(lower, "missing required"):
		return "missing_params"
	case strings.Contains(lower, "unknown tool"):
		return "unknown_tool"
	case strings.Contains(lower, "missing required field"):
		return "missing_field"
	default:
		return "other"
	}
}

// Truncate shortens s to maxLen for logging, appending "..." if trimmed.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("... (%d more bytes)", len(s)-maxLen)
}
