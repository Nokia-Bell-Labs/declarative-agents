// Copyright (c) 2026 Nokia. All rights reserved.

package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// SerializeManifest converts a slice of ToolSpec into prompt text.
// Each tool block uses a Markdown heading, verbatim description, and
// a fenced JSON code block for the schema. The function is pure and
// deterministic: identical input always yields identical output.
func SerializeManifest(specs []core.ToolSpec) string {
	if len(specs) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, s := range specs {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "### %s\n%s", s.Name, s.Description)
		schema := normalizeSchema(s.InputSchema)
		if len(schema) > 0 {
			sb.WriteString("\n\n```json\n")
			sb.Write(schema)
			sb.WriteString("\n```")
		}
	}
	return sb.String()
}

// normalizeSchema re-indents valid JSON to 2-space nesting. Invalid
// JSON is returned as-is. Empty, nil, or "null" returns nil.
func normalizeSchema(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, trimmed, "", "  "); err != nil {
		return trimmed
	}
	return buf.Bytes()
}
