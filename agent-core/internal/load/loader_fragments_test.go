// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Fragments in the closure: usedness of an instantiation and its provenance
// in the dump (srd052 R3).

const echoFragment = `unit: echo-frag
params:
- {name: word, type: string}
tools:
- name: say
  binary: echo
  args: ["$param(word)"]
`

func TestLoadClosureReportsAnUnselectedInstantiationAsUnused(t *testing.T) {
	root := writeUsednessClosureFixture(t, "other")
	writeLoadFixture(t, root, "frag.yaml", echoFragment)
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
instantiate:
- {fragment: frag.yaml, as: hi, args: {word: hello}}
tools:
  - name: other
    binary: echo
`)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, "unused declaration imports")
	require.ErrorContains(t, err, `tool fragment "echo-frag"`)
	require.ErrorContains(t, err, "instantiated with (word=hello)")
	require.ErrorContains(t, err, `by unit "root"`)
}

func TestLoadClosureDumpsEveryInstantiation(t *testing.T) {
	root := writeUsednessClosureFixture(t, "hi_say, bye_say")
	writeLoadFixture(t, root, "frag.yaml", echoFragment)
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
instantiate:
- {fragment: frag.yaml, as: hi, args: {word: hello}}
- {fragment: frag.yaml, as: bye, args: {word: goodbye}}
tools: []
`)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})
	require.NoError(t, err)
	var first, second bytes.Buffer
	require.NoError(t, DumpConfig(closure, &first))
	require.NoError(t, DumpConfig(closure, &second))

	require.Equal(t, first.String(), second.String())
	dump := first.String()
	require.Contains(t, dump, "instantiations:\n")
	require.Contains(t, dump, "as: bye\n    args:\n      word: goodbye\n    produces:\n      - bye_say\n")
	require.Contains(t, dump, "as: hi\n    args:\n      word: hello\n    produces:\n      - hi_say\n")
	require.Less(t, bytes.Index(first.Bytes(), []byte("as: bye")), bytes.Index(first.Bytes(), []byte("as: hi")),
		"entries sort by fragment then prefix")
	require.Contains(t, dump, filepath.Join(root, "frag.yaml"), "the fragment file is in the closure")
}

const clientFragment = `unit: client-frag
params:
- {name: url, type: string}
rest:
  version: v1
  clients:
    api:
      base_url: $param(url)
`

func TestLoadClosureReportsAnUnreferencedRESTInstantiationAsUnused(t *testing.T) {
	root := writeUsednessClosureFixture(t, "other")
	writeLoadFixture(t, root, "declarations.yaml", "tools:\n  - name: other\n    binary: echo\n")
	writeLoadFixture(t, root, "client-frag.yaml", clientFragment)
	writeLoadFixture(t, root, "rest.yaml", `unit: rest-root
instantiate:
- {fragment: client-frag.yaml, as: spare, args: {url: "http://spare"}}
rest: {version: v1}
`)
	writeUsednessProfile(t, root, true)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, "unused declaration imports")
	require.ErrorContains(t, err, `REST fragment "client-frag"`)
	require.ErrorContains(t, err, "instantiated with (url=http://spare)")
}

func TestLoadClosureDumpsRESTInstantiationsBesideToolOnes(t *testing.T) {
	root := writeUsednessClosureFixture(t, "call")
	writeLoadFixture(t, root, "client-frag.yaml", clientFragment)
	writeLoadFixture(t, root, "rest.yaml", `unit: rest-root
instantiate:
- {fragment: client-frag.yaml, as: main, args: {url: "http://main"}}
rest: {version: v1}
`)
	writeLoadFixture(t, root, "declarations.yaml", `tools:
  - name: call
    type: builtin
    init: rest_client_get
    category: boundary
    emits: [ToolDone, CommandError]
    config: {rest_ref: main_api, resource: r, operation: o}
`)
	writeUsednessProfile(t, root, true)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})
	require.NoError(t, err)
	var dump bytes.Buffer
	require.NoError(t, DumpConfig(closure, &dump))

	require.Contains(t, dump.String(), "instantiations:\n")
	require.Contains(t, dump.String(), "as: main\n    args:\n      url: http://main\n    produces:\n      - clients/main_api\n")
}
