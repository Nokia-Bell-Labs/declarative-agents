// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// A word only a request machine runs is used (srd052 R3.1); when that request
// machine's profile does not load, the load reports the profile, not the
// imports it leaves uncounted (GH-2250). The registries are process-scoped,
// so these tests do not run in parallel.

func writeRequestUsednessFixture(t *testing.T, requestProfile string) string {
	t.Helper()
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "declarations.yaml", `unit: host-words
imports: [/opt/agent-core/tools/units/embed-query-words.yaml]
tools:
  - name: selected
    binary: echo
`)
	writeLoadFixture(t, root, "rest.yaml", `unit: host-rest
instantiate:
  - fragment: /opt/providers/embed-query-fragment.yaml
    args: {input_selector: $.text}
rest:
  version: v1
  limits:
    api: {timeout: 5s}
  servers:
    api:
      address: 127.0.0.1:18999
      limits_ref: api
      endpoints:
        ask:
          method: POST
          path: /ask
          binding: machine_request
          machine_request:
            profile: `+requestProfile+`
            timeout: 5s
            response:
              terminal_states:
                Done: {status: 200, content_type: application/json, body: {ok: true}}
`)
	writeLoadFixture(t, root, "request-machine.yaml", `name: request
initial_state: Idle
states: [Idle, {name: Done, run_status: succeeded}, {name: Failed, run_status: failed}]
terminal_states: [Done, Failed]
signals: [Seed, QueryEmbedded, QueryEmbeddingNormalized, ProviderUnauthorized, ProviderThrottled, ProviderUnavailable, CommandError]
transitions: []
instantiate:
  - fragment: /opt/agent-core/tools/machines/embed-query-stage-fragment.yaml
    args: {from: Idle, enter: Seed, next: Done}
`)
	writeLoadFixture(t, root, "request-tools.yaml", "tools: []\n")
	writeLoadFixture(t, root, "request-profile.yaml", `name: request
machine: request-machine.yaml
tools: [request-tools.yaml]
tool_declarations: [declarations.yaml]
libraries: {providers: `+filepath.Join(agentCoreRoot(t), "tools", "providers", "ollama")+`}
`)
	writeLoadFixture(t, root, "profile.yaml", `name: usedness
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
rest_definitions: [rest.yaml]
libraries: {providers: `+filepath.Join(agentCoreRoot(t), "tools", "providers", "ollama")+`}
`)
	return filepath.Join(root, "profile.yaml")
}

func TestRequestMachineWordsCountAsUsed(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetInstallRoot("") })
	corepath.SetInstallRoot(agentCoreRoot(t))

	_, err := LoadClosure(writeRequestUsednessFixture(t, "request-profile.yaml"), Options{})

	require.NoError(t, err, "the embed words and client only the request machine runs are used")
}

func TestUnloadableRequestProfileIsReportedNotItsStrandedImports(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetInstallRoot("") })
	corepath.SetInstallRoot(agentCoreRoot(t))

	_, err := LoadClosure(writeRequestUsednessFixture(t, "missing-request-profile.yaml"), Options{})

	require.ErrorContains(t, err, "machine_request machine does not load")
	require.ErrorContains(t, err, "missing-request-profile.yaml", "the error names the request profile")
	require.NotContains(t, err.Error(), "unused declaration imports")
}
