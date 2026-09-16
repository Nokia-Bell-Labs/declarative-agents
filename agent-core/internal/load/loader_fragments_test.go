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
