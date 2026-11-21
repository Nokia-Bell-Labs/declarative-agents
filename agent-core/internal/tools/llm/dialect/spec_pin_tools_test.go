// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dialect

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Pinning test for application-dsl-spec 1.0 R-LLM-002: the chat dialect's
// header names a kebab unit and a provider, a rooted POST path, and an auth
// type.
func TestSpecPinChatDialectHeader(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ old, replacement, want string }{
		"unit":          {"unit: fixture-chat-dialect", "unit: Fixture_Dialect", "lowercase kebab"},
		"provider name": {"provider_name: fixture", `provider_name: ""`, "provider_name is required"},
		"path":          {"path: /v1/chat", "path: v1/chat", "must start with /"},
		"method":        {"path: /v1/chat", "method: GET\npath: /v1/chat", "a chat call is a POST"},
		"auth type":     {"  type: bearer\n", "", "auth.type is required"},
	}
	for name, edit := range cases {
		text := strings.Replace(fixtureText(t), edit.old, edit.replacement, 1)

		_, err := Parse([]byte(text))

		require.ErrorContains(t, err, edit.want, name)
	}
}
