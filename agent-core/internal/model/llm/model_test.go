// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestModelInfo_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	m := ModelInfo{
		Name:       "llama3:latest",
		Size:       4_600_000_000,
		ModifiedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		Provider:   "ollama",
		Details: map[string]string{
			"parameter_size":     "8.0B",
			"quantization_level": "Q4_K_M",
			"family":             "llama",
		},
	}

	data, err := json.Marshal(m)
	require.NoError(t, err)

	var decoded ModelInfo
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, m.Name, decoded.Name)
	require.Equal(t, m.Size, decoded.Size)
	require.Equal(t, m.Provider, decoded.Provider)
	require.True(t, m.ModifiedAt.Equal(decoded.ModifiedAt))
	require.Equal(t, "8.0B", decoded.Details["parameter_size"])
	require.Equal(t, "Q4_K_M", decoded.Details["quantization_level"])
	require.Equal(t, "llama", decoded.Details["family"])
}

func TestModelInfo_JSONFieldNames(t *testing.T) {
	t.Parallel()
	m := ModelInfo{
		Name:     "gpt-4",
		Provider: "openai",
	}
	data, err := json.Marshal(m)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Equal(t, "gpt-4", raw["name"])
	require.Equal(t, "openai", raw["provider"])
	require.Contains(t, raw, "modified_at")
}

func TestModelInfo_DetailsOmittedWhenNil(t *testing.T) {
	t.Parallel()
	m := ModelInfo{Name: "test", Provider: "local"}
	data, err := json.Marshal(m)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	_, hasDetails := raw["details"]
	require.False(t, hasDetails, "details should be omitted when nil")
}

func TestModelInfo_EmptyDetails(t *testing.T) {
	t.Parallel()
	m := ModelInfo{Name: "test", Details: map[string]string{}}
	data, err := json.Marshal(m)
	require.NoError(t, err)

	var decoded ModelInfo
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Empty(t, decoded.Details)
}
