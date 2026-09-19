// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dialect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderBodyPlacesTheConversationAsAnArray(t *testing.T) {
	t.Parallel()
	chat, err := Load(fixturePath)
	require.NoError(t, err)
	messages := []interface{}{
		map[string]interface{}{"role": "system", "content": "be brief"},
		map[string]interface{}{"role": "user", "content": "hi"},
	}

	body, err := chat.RenderBody(map[string]interface{}{
		ParamModel: "m1", ParamMessages: messages, ParamTemperature: 0.0, ParamSeed: 42,
	})

	require.NoError(t, err)
	require.Equal(t, messages, body["messages"], "a whole-scalar reference takes the value whole")
	require.Equal(t, "m1", body["model"])
	require.Equal(t, false, body["stream"])
	require.Equal(t, 42, body["seed"])
	require.Equal(t, map[string]interface{}{}, body["options"],
		"an option the call does not supply is omitted, not sent as null")
}

func TestRenderBodyInterpolatesInsideText(t *testing.T) {
	t.Parallel()
	chat := Chat{Body: map[string]interface{}{"model": "family/{{ params.model }}"}}

	body, err := chat.RenderBody(map[string]interface{}{ParamModel: "m1"})
	require.NoError(t, err)
	require.Equal(t, "family/m1", body["model"])

	_, err = chat.RenderBody(map[string]interface{}{})
	require.ErrorContains(t, err, "params.model")
}
