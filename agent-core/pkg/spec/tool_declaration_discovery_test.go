// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Declaration discovery follows what declarations name (GH-2330). Before this,
// the corpus read a profile's tool_declarations and a module's own tools/
// tree, which left two ways for a declared word to be invisible: a word that
// runs a nested machine names that machine's vocabulary in its own config, and
// an agent directory with a machine but no profile.yaml has nothing to name
// the declarations it ships. Both reported every affected word as undeclared
// while the runtime loaded them.

// stageDiscoveryCorpus writes the smallest corpus that exercises both paths: a
// profile-less agent shipping declarations.yaml, and a profile whose word
// names a further declaration file in its config.
func stageDiscoveryCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}

	write("docs/road-map.yaml", "id: fixture\ntitle: Fixture\nreleases: []\n")
	write("docs/SPECIFICATIONS.yaml", "id: fixture-specifications\ntitle: Fixture\n")
	// LoadCorpus requires at least one SRD; the corpus under test here is the
	// declaration vocabulary, so one minimal document is enough.
	write("docs/specs/software-requirements/srd001-fixture.yaml", `id: srd001-fixture
title: Fixture
requirements:
  R1:
    title: Fixture
    items:
      - R1.1: The fixture declares words.
acceptance_criteria:
  - id: AC1
    criterion: The fixture declares words.
    traces: [R1.1]
`)

	// A profile-less family fragment: machine.yaml makes it an agent
	// directory, and declarations.yaml is the only place its word is written.
	write("agents/fragment/machine.yaml", `name: fragment
initial_state: Idle
budget: {max_iterations: 10, command_timeout: 30s}
states:
- {name: Idle, meaning: Waiting.}
- {name: Running, meaning: Working.}
- {name: Done, meaning: Terminal success., run_status: succeeded}
- {name: Failed, meaning: Terminal failure., run_status: failed}
terminal_states: [Done, Failed]
signals:
- {name: Seed, trigger: Loop initialization.}
- {name: ToolDone, trigger: A tool completed.}
- {name: CommandError, trigger: Infrastructure error.}
transitions:
- {state: Idle, signal: Seed, next: Running, action: fragment_word}
- {state: Running, signal: ToolDone, next: Done}
- {state: Running, signal: CommandError, next: Failed}
`)
	write("agents/fragment/tools.yaml", "tools:\n  - fragment_word\n")
	write("agents/fragment/declarations.yaml", `unit: fixture-fragment
tools:
  - name: fragment_word
    type: builtin
    init: compose
    category: response
    visibility: internal
    signature: {emits: [ToolDone]}
    description: A word declared beside a machine with no profile.
    parameters: {type: object, properties: {}, additionalProperties: false}
    output: {description: Fixture output., schema: {type: string}}
`)

	// A profile whose word names a nested machine's vocabulary in its config.
	write("agents/host/profile.yaml", "name: host\nmachine: machine.yaml\ntools:\n  - tools.yaml\ntool_declarations:\n  - declarations.yaml\n")
	write("agents/host/machine.yaml", `name: host
initial_state: Idle
budget: {max_iterations: 10, command_timeout: 30s}
states:
- {name: Idle, meaning: Waiting.}
- {name: Running, meaning: Working.}
- {name: Done, meaning: Terminal success., run_status: succeeded}
- {name: Failed, meaning: Terminal failure., run_status: failed}
terminal_states: [Done, Failed]
signals:
- {name: Seed, trigger: Loop initialization.}
- {name: ToolDone, trigger: A tool completed.}
- {name: CommandError, trigger: Infrastructure error.}
transitions:
- {state: Idle, signal: Seed, next: Running, action: run_nested}
- {state: Running, signal: ToolDone, next: Done}
- {state: Running, signal: CommandError, next: Failed}
`)
	write("agents/host/tools.yaml", "tools:\n  - run_nested\n  - nested_word\n")
	write("agents/host/declarations.yaml", `unit: fixture-host
tools:
  - name: run_nested
    type: builtin
    init: compose
    category: response
    visibility: internal
    signature: {emits: [ToolDone]}
    description: Runs a nested machine whose vocabulary its config names.
    parameters: {type: object, properties: {}, additionalProperties: false}
    output: {description: Fixture output., schema: {type: string}}
    config:
      point_tool_declarations:
        - agents/host/nested-declarations.yaml
`)
	write("agents/host/nested-declarations.yaml", `unit: fixture-nested
tools:
  - name: nested_word
    type: builtin
    init: compose
    category: response
    visibility: internal
    signature: {emits: [ToolDone]}
    description: A word only the nested-machine config names.
    parameters: {type: object, properties: {}, additionalProperties: false}
    output: {description: Fixture output., schema: {type: string}}
`)
	return root
}

func TestCorpusReadsDeclarationsNamedByAToolConfig(t *testing.T) {
	corpus, err := LoadCorpus(stageDiscoveryCorpus(t))
	require.NoError(t, err)

	_, ok := corpus.ToolDeclarations["nested_word"]
	require.True(t, ok,
		"a word named only by another word's *_tool_declarations config is absent from the corpus")
}

func TestCorpusReadsDeclarationsShippedBesideAProfilelessMachine(t *testing.T) {
	corpus, err := LoadCorpus(stageDiscoveryCorpus(t))
	require.NoError(t, err)

	_, ok := corpus.ToolDeclarations["fragment_word"]
	require.True(t, ok,
		"a word declared beside a machine with no profile.yaml is absent from the corpus")
}

// The selections and the declarations have to agree, which is the finding the
// discovery gap produced: every word the fixture selects is declared, so
// validateToolCorpus reports nothing.
func TestDiscoveredDeclarationsSatisfyToolSelections(t *testing.T) {
	corpus, err := LoadCorpus(stageDiscoveryCorpus(t))
	require.NoError(t, err)

	for _, finding := range validateToolCorpus(corpus) {
		if finding.Check == "tool-selection-undeclared" {
			t.Errorf("unexpected finding: %s", finding.Message)
		}
	}
}

// A config naming a path the corpus cannot see is not a load failure: the
// unresolved-declaration check reports it, and the rest of the corpus loads.
func TestCorpusToleratesAToolConfigNamingAMissingDeclaration(t *testing.T) {
	root := stageDiscoveryCorpus(t)
	require.NoError(t, os.Remove(filepath.Join(root, "agents", "host", "nested-declarations.yaml")))

	corpus, err := LoadCorpus(root)
	require.NoError(t, err, "a missing config-named declaration must not fail the load")
	_, ok := corpus.ToolDeclarations["run_nested"]
	require.True(t, ok, "the naming word itself stays in the corpus")
}
