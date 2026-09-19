// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dialect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var fixturePath = filepath.Join("..", "..", "..", "..", "testdata", "providers", "fixture", FileName)

func fixtureText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	return string(data)
}

func TestChatDialectLoadsStrictly(t *testing.T) {
	t.Parallel()
	chat, err := Load(fixturePath)

	require.NoError(t, err)
	require.Equal(t, "fixture-chat-dialect", chat.Unit)
	require.Equal(t, "POST", chat.Method, "a chat call defaults to POST")
	require.Equal(t, "FIXTURE_API_KEY", chat.Auth.TokenRef)
	require.Equal(t, ProviderThrottled, chat.FailureSignal(429))
	require.Equal(t, ProviderUnavailable, chat.FailureSignal(503))
	require.Empty(t, chat.FailureSignal(404), "an unmapped status names no signal")
}

func TestChatDialectRejectsUnknownField(t *testing.T) {
	t.Parallel()
	_, err := Parse([]byte(fixtureText(t) + "stream_mode: sse\n"))

	require.ErrorContains(t, err, "stream_mode")
}

func TestChatDialectRejectsCredentialLiteral(t *testing.T) {
	t.Parallel()
	for name, replacement := range map[string]string{
		"value in token_ref": "token_ref: sk-live-0123456789",
		"inline token field": "token: sk-live-0123456789",
	} {
		text := strings.Replace(fixtureText(t), "token_ref: FIXTURE_API_KEY", replacement, 1)

		_, err := Parse([]byte(text))

		require.Error(t, err, name)
	}
}

func TestChatDialectRejectsMessagesInsideString(t *testing.T) {
	t.Parallel()
	text := strings.Replace(fixtureText(t),
		`messages: "{{ params.messages }}"`, `messages: "history: {{ params.messages }}"`, 1)

	_, err := Parse([]byte(text))

	require.ErrorContains(t, err, "whole scalar")
}

func TestChatDialectRejectsInvalidDeclarations(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"unsupplied param":     {`seed: "{{ params.seed }}"`, `seed: "{{ params.random }}"`},
		"foreign signal":       {"signal: ProviderThrottled", "signal: RESTThrottled"},
		"success status":       {"status: [429]", "status: [200]"},
		"duplicate status":     {"status: [429]", "status: [401]"},
		"none with credential": {"type: bearer", "type: none"},
		"missing text":         {"text: $.message.content.type=text.text", "text: \"\""},
		"bad selector":         {"text: $.message.content.type=text.text", "text: message.content"},
		"unknown parser":       {"failures:", "parser_profile: nowhere\nfailures:"},
		"nameless profile":     {"failures:", "parser_profiles:\n- {match_prefixes: [x-]}\nfailures:"},
	}
	for name, edit := range cases {
		text := strings.Replace(fixtureText(t), edit[0], edit[1], 1)

		_, err := Parse([]byte(text))

		require.Error(t, err, name)
	}
}

func TestChatDialectLoadNamesThePath(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), FileName)

	_, err := Load(missing)

	require.ErrorContains(t, err, missing)
}
