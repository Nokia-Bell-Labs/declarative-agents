// Copyright (c) 2026 Nokia. All rights reserved.

package prompt

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// ---------------------------------------------------------------------------
// RenderSystemPrompt
// ---------------------------------------------------------------------------

func TestRender_RoleAndTask(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role: "You are a helper.",
		Task: "Do the thing.",
	})
	require.Contains(t, out, "You are a helper.")
	require.Contains(t, out, "Do the thing.")
}

func TestRender_ConstraintsIncluded(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role:        "role",
		Task:        "task",
		Constraints: "no bad things",
	})
	require.Contains(t, out, "no bad things")
}

func TestRender_ConstraintsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{Role: "role", Task: "task"})
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		require.NotContains(t, l, "Constraints")
	}
}

func TestRender_OutputFormatIncluded(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role:         "role",
		Task:         "task",
		OutputFormat: "return JSON",
	})
	require.Contains(t, out, "return JSON")
}

func TestRender_ToolManifestSection(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role:         "role",
		Task:         "task",
		ToolManifest: "### read\nRead a file.",
	})
	require.Contains(t, out, "## Available Tools")
	require.Contains(t, out, "### read")
	require.Contains(t, out, "Read a file.")
	require.Contains(t, out, `"tool": "<tool_name>"`)
}

func TestRender_ToolManifestOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{Role: "role", Task: "task"})
	require.NotContains(t, out, "Available Tools")
}

func TestRender_WithEnvelope(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role:         "role",
		Task:         "task",
		ToolManifest: "### tool\ndesc",
		Envelope:     &Envelope{Open: "<tool>", Close: "</tool>"},
	})
	require.Contains(t, out, "<tool>")
	require.Contains(t, out, "</tool>")
	require.Contains(t, out, "Wrap it in <tool> / </tool> tags")
}

func TestRender_WithoutEnvelope_BareJSON(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role:         "role",
		Task:         "task",
		ToolManifest: "### tool\ndesc",
	})
	require.NotContains(t, out, "Wrap it in")
	require.Contains(t, out, `"tool": "<tool_name>"`)
}

func TestRender_StrictFormat(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role:         "role",
		Task:         "task",
		ToolManifest: "### tool\ndesc",
		StrictFormat: true,
	})
	require.Contains(t, out, "Do not include any other text")
}

func TestRender_StrictFormatOmittedWhenFalse(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role:         "role",
		Task:         "task",
		ToolManifest: "### tool\ndesc",
		StrictFormat: false,
	})
	require.NotContains(t, out, "Do not include any other text")
}

func TestRender_Deterministic(t *testing.T) {
	t.Parallel()
	data := PromptData{
		Role:         "role",
		Task:         "task",
		Constraints:  "c",
		OutputFormat: "o",
		ToolManifest: "### t\nd",
		Envelope:     &Envelope{Open: "<a>", Close: "</a>"},
		StrictFormat: true,
	}
	a := RenderSystemPrompt(data)
	b := RenderSystemPrompt(data)
	require.Equal(t, a, b)
}

func TestRender_NoTrailingNewlines(t *testing.T) {
	t.Parallel()
	out := RenderSystemPrompt(PromptData{
		Role:         "role",
		Task:         "task",
		ToolManifest: "### tool\ndesc",
	})
	require.False(t, strings.HasSuffix(out, "\n"))
}

// ---------------------------------------------------------------------------
// RenderSystemPromptWith (custom template)
// ---------------------------------------------------------------------------

func TestRenderWith_CustomTemplate(t *testing.T) {
	t.Parallel()
	tmpl := `ROLE: {{.Role}} | TASK: {{.Task}}`
	out, err := RenderSystemPromptWith(tmpl, PromptData{Role: "r", Task: "t"})
	require.NoError(t, err)
	require.Equal(t, "ROLE: r | TASK: t", out)
}

func TestRenderWith_InvalidTemplate(t *testing.T) {
	t.Parallel()
	_, err := RenderSystemPromptWith("{{.Bad", PromptData{})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// SerializeManifest
// ---------------------------------------------------------------------------

func TestSerializeManifest_Empty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", SerializeManifest(nil))
	require.Equal(t, "", SerializeManifest([]core.ToolSpec{}))
}

func TestSerializeManifest_SingleToolNoSchema(t *testing.T) {
	t.Parallel()
	specs := []core.ToolSpec{
		{Name: "done", Description: "Signal completion."},
	}
	out := SerializeManifest(specs)
	require.Equal(t, "### done\nSignal completion.", out)
}

func TestSerializeManifest_SingleToolWithSchema(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	specs := []core.ToolSpec{
		{Name: "read", Description: "Read a file.", InputSchema: schema},
	}
	out := SerializeManifest(specs)
	require.Contains(t, out, "### read\nRead a file.")
	require.Contains(t, out, "```json\n")
	require.Contains(t, out, `"type": "object"`)
	require.Contains(t, out, "\n```")
}

func TestSerializeManifest_MultipleTools(t *testing.T) {
	t.Parallel()
	specs := []core.ToolSpec{
		{Name: "read", Description: "Read a file."},
		{Name: "write", Description: "Write a file."},
		{Name: "done", Description: "Complete."},
	}
	out := SerializeManifest(specs)

	parts := strings.Split(out, "\n\n")
	require.GreaterOrEqual(t, len(parts), 3)
	require.True(t, strings.HasPrefix(out, "### read"))
	require.Contains(t, out, "### write")
	require.Contains(t, out, "### done")
}

func TestSerializeManifest_PreservesOrder(t *testing.T) {
	t.Parallel()
	specs := []core.ToolSpec{
		{Name: "beta", Description: "B"},
		{Name: "alpha", Description: "A"},
	}
	out := SerializeManifest(specs)
	idxBeta := strings.Index(out, "### beta")
	idxAlpha := strings.Index(out, "### alpha")
	require.Less(t, idxBeta, idxAlpha, "order must match input slice, not sorted")
}

func TestSerializeManifest_Deterministic(t *testing.T) {
	t.Parallel()
	specs := []core.ToolSpec{
		{Name: "a", Description: "da", InputSchema: json.RawMessage(`{"x":1}`)},
		{Name: "b", Description: "db"},
	}
	a := SerializeManifest(specs)
	b := SerializeManifest(specs)
	require.Equal(t, a, b)
}

func TestSerializeManifest_NoLeadingBlankLine(t *testing.T) {
	t.Parallel()
	specs := []core.ToolSpec{{Name: "t", Description: "d"}}
	out := SerializeManifest(specs)
	require.False(t, strings.HasPrefix(out, "\n"))
}

// ---------------------------------------------------------------------------
// normalizeSchema
// ---------------------------------------------------------------------------

func TestNormalizeSchema_ReIndents(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"a":1,"b":{"c":2}}`)
	got := normalizeSchema(raw)
	require.Contains(t, string(got), "  \"a\": 1")
	require.Contains(t, string(got), "    \"c\": 2")
}

func TestNormalizeSchema_NilReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, normalizeSchema(nil))
}

func TestNormalizeSchema_EmptyReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, normalizeSchema(json.RawMessage{}))
}

func TestNormalizeSchema_NullReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, normalizeSchema(json.RawMessage(`null`)))
}

func TestNormalizeSchema_InvalidJSON_ReturnsRaw(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{not valid}`)
	got := normalizeSchema(raw)
	require.Equal(t, []byte(`{not valid}`), got)
}

func TestNormalizeSchema_WhitespaceOnlyReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, normalizeSchema(json.RawMessage("   \n  ")))
}
