// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// Machine templates in the closure (srd054 R3.1, R3.2): the template is a file
// of the closure and an asset of the program digest, and the dump labels the
// expansion.

const usednessTemplate = `unit: usedness-template
params:
- {name: word, type: string}
machine:
  name: usedness
  initial_state: Idle
  states: [Idle, {name: Done, run_status: succeeded}]
  terminal_states: [Done]
  signals: [Seed]
  transitions: [{state: Idle, signal: Seed, next: Done, action: $param(word)}]
`

func writeTemplateClosureFixture(t *testing.T) string {
	t.Helper()
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "declarations.yaml", "tools:\n  - name: selected\n    binary: echo\n")
	writeLoadFixture(t, root, "template.yaml", usednessTemplate)
	writeLoadFixture(t, root, "machine.yaml",
		"unit: usedness-instance\nname: usedness-instance\nexpand:\n  - {fragment: template.yaml, args: {word: selected}}\n")
	return root
}

func TestLoadClosureCarriesTheMachineTemplate(t *testing.T) {
	root := writeTemplateClosureFixture(t)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})
	require.NoError(t, err)

	require.Equal(t, "usedness-instance", closure.Machine.Name)
	require.Contains(t, closure.Files, filepath.Join(root, "template.yaml"), "the template is a closure file")
	var dump bytes.Buffer
	require.NoError(t, DumpConfig(closure, &dump))
	require.Contains(t, dump.String(), "  - kind: machine\n    fragment: "+filepath.Join(root, "template.yaml")+
		"\n    args:\n      word: selected\n    produces:\n      - machine/usedness-instance\n")

	before := catalog.BuildProgramRefFromAssets(closure.ProfilePath, closure.Assets)
	writeLoadFixture(t, root, "template.yaml", usednessTemplate+"  purpose: Changed.\n")
	changed, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})
	require.NoError(t, err)
	after := catalog.BuildProgramRefFromAssets(changed.ProfilePath, changed.Assets)
	require.Equal(t, before.Profile, after.Profile, "the instance stays the program's identity")
	require.NotEqual(t, before.Digest, after.Digest, "changing a template changes its instances' digest")
}

func TestLoadClosureRefusesAProfileNamingAMachineTemplate(t *testing.T) {
	root := writeTemplateClosureFixture(t)
	profile, err := os.ReadFile(filepath.Join(root, "profile.yaml"))
	require.NoError(t, err)
	writeLoadFixture(t, root, "profile.yaml", string(bytes.Replace(profile,
		[]byte("machine: machine.yaml"), []byte("machine: template.yaml"), 1)))

	_, err = LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, "is a machine template")
	require.ErrorContains(t, err, filepath.Join(root, "template.yaml"))
}
