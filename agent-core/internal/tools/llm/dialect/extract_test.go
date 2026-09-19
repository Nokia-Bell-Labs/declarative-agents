// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dialect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractReplyJoinsSelectedTextAndReadsUsage(t *testing.T) {
	t.Parallel()
	chat, err := Load(fixturePath)
	require.NoError(t, err)
	body := []byte(`{"message":{"content":[
		{"type":"thinking","thinking":"hidden"},
		{"type":"text","text":"Hello, "},
		{"type":"text","text":"world"}]},
		"usage":{"tokens":{"input_tokens":12,"output_tokens":3}}}`)

	reply, err := chat.ExtractReply(body)

	require.NoError(t, err)
	require.Equal(t, Reply{Text: "Hello, world", TokensIn: 12, TokensOut: 3}, reply)
}

func TestExtractReplyRequiresTextWhenDeclared(t *testing.T) {
	t.Parallel()
	chat, err := Load(fixturePath)
	require.NoError(t, err)

	_, err = chat.ExtractReply([]byte(`{"message":{"content":[]}}`))
	require.ErrorContains(t, err, "no text")

	chat.Response.TextRequired = false
	reply, err := chat.ExtractReply([]byte(`{"message":{"content":[]}}`))
	require.NoError(t, err)
	require.Empty(t, reply.Text)
}

func TestExtractReplyWalksIndexesAndPlainPaths(t *testing.T) {
	t.Parallel()
	chat := Chat{ProviderName: "p", Response: Response{
		Text: "$.choices.0.message.content", InputTokens: "$.usage.prompt_tokens",
	}}

	reply, err := chat.ExtractReply([]byte(`{"choices":[{"message":{"content":"first"}},{"message":{"content":"second"}}],"usage":{"prompt_tokens":7}}`))

	require.NoError(t, err)
	require.Equal(t, Reply{Text: "first", TokensIn: 7}, reply)

	_, err = chat.ExtractReply([]byte(`not json`))
	require.ErrorContains(t, err, "decode p response")

	chat.Response.InputTokens = "$.usage"
	_, err = chat.ExtractReply([]byte(`{"choices":[],"usage":{"prompt_tokens":7}}`))
	require.ErrorContains(t, err, "not a number")
}
