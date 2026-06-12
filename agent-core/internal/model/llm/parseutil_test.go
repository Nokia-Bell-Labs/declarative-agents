// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixNewlinesInStrings_LiteralNewlines(t *testing.T) {
	input := `{"tool":"write","parameters":{"content":"line1` + "\n" + `line2"}}`
	fixed := FixNewlinesInStrings(input)
	var v map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(fixed), &v))
}

func TestFixNewlinesInStrings_EscapedNewlines(t *testing.T) {
	input := `{"tool":"write","parameters":{"content":"line1\nline2"}}`
	assert.Equal(t, input, FixNewlinesInStrings(input))
}

func TestFixNewlinesInStrings_Tabs(t *testing.T) {
	input := `{"content":"has` + "\t" + `tab"}`
	fixed := FixNewlinesInStrings(input)
	assert.Contains(t, fixed, `\t`)
	var v map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(fixed), &v))
}

func TestExtractFlatParams(t *testing.T) {
	result := ExtractFlatParams(`{"tool":"edit","path":"f.go","old_string":"x"}`, "edit")
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &m))
	assert.Equal(t, "f.go", m["path"])
	assert.Equal(t, "x", m["old_string"])
	_, hasTool := m["tool"]
	assert.False(t, hasTool)
}

func TestExtractFlatParams_Empty(t *testing.T) {
	result := ExtractFlatParams(`{"tool":"build"}`, "build")
	assert.Equal(t, `{}`, string(result))
}

func TestExtractFlatParams_InvalidJSON(t *testing.T) {
	result := ExtractFlatParams(`not json`, "")
	assert.Equal(t, `{}`, string(result))
}

func TestCountToolCallBlocks(t *testing.T) {
	assert.Equal(t, 0, CountToolCallBlocks(`{"tool":"read"}`))
	assert.Equal(t, 1, CountToolCallBlocks(`[tool_call]{"tool":"read"}[/tool_call]`))
	assert.Equal(t, 2, CountToolCallBlocks(`[tool_call]{"tool":"read"}[/tool_call] text [tool_call]{"tool":"write"}[/tool_call]`))
}

func TestEstimateTokens(t *testing.T) {
	msgs := []Message{
		{Role: User, Content: "hello world twelve chars"},
	}
	est := EstimateTokens(msgs)
	assert.Equal(t, len("hello world twelve chars")/4, est)
}

func TestExtractDoneSummary(t *testing.T) {
	assert.Equal(t, "all done", ExtractDoneSummary(json.RawMessage(`{"summary":"all done"}`)))
	assert.Equal(t, `{"other":"field"}`, ExtractDoneSummary(json.RawMessage(`{"other":"field"}`)))
	assert.Equal(t, `{}`, ExtractDoneSummary(json.RawMessage(`{}`)))
}

func TestCheckRequiredFields_AllPresent(t *testing.T) {
	schema := json.RawMessage(`{"required":["path","content"]}`)
	params := json.RawMessage(`{"path":"f.go","content":"x"}`)
	assert.Empty(t, CheckRequiredFields(schema, params))
}

func TestCheckRequiredFields_Missing(t *testing.T) {
	schema := json.RawMessage(`{"required":["path","content"]}`)
	params := json.RawMessage(`{"path":"f.go"}`)
	missing := CheckRequiredFields(schema, params)
	assert.Equal(t, []string{"content"}, missing)
}

func TestCheckRequiredFields_NoRequired(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	params := json.RawMessage(`{}`)
	assert.Empty(t, CheckRequiredFields(schema, params))
}

func TestCheckRequiredFields_EmptySchema(t *testing.T) {
	assert.Empty(t, CheckRequiredFields(nil, json.RawMessage(`{}`)))
}

func TestClassifyParseError(t *testing.T) {
	assert.Equal(t, "malformed_json", ClassifyParseError("malformed JSON: unexpected EOF"))
	assert.Equal(t, "unknown_tool", ClassifyParseError(`unknown tool "foo"`))
	assert.Equal(t, "missing_params", ClassifyParseError("missing required parameters: [path]"))
	assert.Equal(t, "other", ClassifyParseError("something else went wrong"))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "short", Truncate("short", 100))
	result := Truncate("this is a longer string", 10)
	assert.Equal(t, 10, len("this is a "))
	assert.Contains(t, result, "this is a ")
	assert.Contains(t, result, "more bytes")
}
