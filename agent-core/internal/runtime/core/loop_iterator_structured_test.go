// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIteratorJoinStructuredOutputJSONKinds(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		expected     interface{}
	}{
		{"object", `{"mapped":{"value":"ok"}}`, map[string]interface{}{"mapped": map[string]interface{}{"value": "ok"}}},
		{"array", `["a","b"]`, []interface{}{"a", "b"}},
		{"scalar", `"value"`, "value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			joined := joinedOutcomeResult(t, DigestResult(Result{Signal: "Done", Output: tc.output}))
			require.Equal(t, tc.expected, joined["structured_output"])
			require.Equal(t, tc.output, joined["output"], "compatibility output remains present")
		})
	}
}

func TestIteratorJoinStructuredOutputOmitsPlainText(t *testing.T) {
	joined := joinedOutcomeResult(t, DigestResult(Result{Signal: "Done", Output: "plain text"}))
	require.Equal(t, "plain text", joined["output"])
	require.NotContains(t, joined, "structured_output")
}

func TestIteratorJoinStructuredOutputUsesRedactedDigest(t *testing.T) {
	digest := DigestResult(Result{
		Signal: "Done",
		Output: `{"mapped":{"visible":"ok","secret":"remove"}}`,
		Redaction: OutputRedaction{
			Version: OutputRedactionVersion1,
			Paths:   []OutputRedactionPath{{"mapped", "secret"}},
		},
	})
	joined := joinedOutcomeResult(t, digest)
	require.JSONEq(t, `{"mapped":{"visible":"ok"}}`, joined["output"].(string))
	require.Equal(t,
		map[string]interface{}{"mapped": map[string]interface{}{"visible": "ok"}},
		joined["structured_output"],
	)
	require.NotContains(t, joined["output"], "remove")
}

func TestIteratorJoinStructuredOutputFailsClosedWithDigest(t *testing.T) {
	digest := DigestResult(Result{
		Signal: "Done",
		Output: `{"mapped":{"value":"secret"}}`,
		Redaction: OutputRedaction{
			Version: OutputRedactionVersion1,
			Paths:   []OutputRedactionPath{{"mapped", "value", "nested"}},
		},
	})
	require.Equal(t, OutputRedactionOmitted, digest.RedactionStatus)
	joined := joinedOutcomeResult(t, digest)
	require.NotContains(t, joined, "output")
	require.NotContains(t, joined, "structured_output")
	encoded, err := json.Marshal(joined)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret")
}

func TestIteratorJoinStructuredOutputResumeDeterminism(t *testing.T) {
	frame := IteratorSnapshot{
		Items: []json.RawMessage{json.RawMessage(`{"name":"a"}`)},
		Spec: ForEachSpec{
			ContinueOn: []string{"Done"},
			Join:       JoinSpec{Signals: JoinSignalSpec{AllSuccess: "Joined"}},
		},
		Outcomes: []IteratorOutcome{{
			Index: 0, Input: json.RawMessage(`{"name":"a"}`), CommandName: "item",
			Result: DigestResult(Result{Signal: "Done", Output: `{"mapped":{"value":"ok"}}`}),
		}},
	}
	before := iteratorJoinResult(&frame)

	snapshot, err := json.Marshal(frame)
	require.NoError(t, err)
	require.NotContains(t, string(snapshot), "structured_output",
		"the projection must not expand persisted checkpoint state")
	var restored IteratorSnapshot
	require.NoError(t, json.Unmarshal(snapshot, &restored))
	after := iteratorJoinResult(&restored)

	require.Equal(t, before.Signal, after.Signal)
	require.JSONEq(t, before.Output, after.Output)
	require.Contains(t, after.Output, `"structured_output":{"mapped":{"value":"ok"}}`)
}

func joinedOutcomeResult(t *testing.T, digest ResultDigest) map[string]interface{} {
	t.Helper()
	frame := IteratorSnapshot{
		Items: []json.RawMessage{json.RawMessage(`{"name":"a"}`)},
		Spec: ForEachSpec{
			ContinueOn: []string{"Done"},
			Join:       JoinSpec{Signals: JoinSignalSpec{AllSuccess: "Joined"}},
		},
		Outcomes: []IteratorOutcome{{
			Index: 0, Input: json.RawMessage(`{"name":"a"}`), CommandName: "item", Result: digest,
		}},
	}
	var output struct {
		Items []map[string]interface{} `json:"items"`
	}
	result := iteratorJoinResult(&frame)
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Len(t, output.Items, 1)
	joined, ok := output.Items[0]["result"].(map[string]interface{})
	require.True(t, ok)
	return joined
}
