// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// The embed and rerank stage fragments splice with every shipped provider
// library (srd058 R4.1). The importer instantiates a stage from agent-core,
// imports its words, and instantiates the bound library's REST fragment; the
// spliced machine passes the checks a hand-written one does. The root
// registries are process-scoped, so these tests do not run in parallel.

type providerStage struct {
	stage, words, fragment string
	signals                []string
	selection              []string
	args                   string
}

var providerStages = map[string]providerStage{
	"embed-query": {
		stage: "embed-query-stage-fragment.yaml", words: "embed-query-words.yaml", fragment: "embed-query-fragment.yaml",
		signals:   []string{"QueryEmbedded", "QueryEmbeddingNormalized"},
		selection: []string{"embed_query", "normalize_query_embedding"},
		args:      "input_selector: $.text, body_source: previous_result",
	},
	"embed-document": {
		stage: "embed-document-stage-fragment.yaml", words: "embed-document-words.yaml", fragment: "embed-document-fragment.yaml",
		signals:   []string{"DocumentEmbedded", "DocumentEmbeddingNormalized"},
		selection: []string{"embed_document", "normalize_document_embedding"},
		args:      "input_selector: $.text, body_source: previous_result",
	},
	"rerank": {
		stage: "rerank-stage-fragment.yaml", words: "rerank-words.yaml", fragment: "rerank-fragment.yaml",
		signals:   []string{"Reranked"},
		selection: []string{"rerank"},
		args:      "query_selector: $.query, documents_selector: $.documents, body_source: previous_result",
	},
}

func agentCoreRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

// writeProviderStageProfile writes a profile whose machine is one provider
// stage between Idle and Done, bound to library.
func writeProviderStageProfile(t *testing.T, stage providerStage, library string) string {
	t.Helper()
	root := t.TempDir()
	signals := "[Seed, ProviderUnauthorized, ProviderThrottled, ProviderUnavailable, CommandError"
	for _, signal := range stage.signals {
		signals += ", " + signal
	}
	writeLoadFixture(t, root, "machine.yaml", fmt.Sprintf(`name: provider-stage
initial_state: Idle
states: [Idle, {name: Done, run_status: succeeded}, {name: Failed, run_status: failed}]
terminal_states: [Done, Failed]
signals: %s]
transitions: []
expand:
  - fragment: /opt/agent-core/tools/machines/%s
    args: {from: Idle, enter: Seed, next: Done}
`, signals, stage.stage))
	selection := "tools: ["
	for index, word := range stage.selection {
		if index > 0 {
			selection += ", "
		}
		selection += word
	}
	writeLoadFixture(t, root, "tools.yaml", selection+"]\n")
	writeLoadFixture(t, root, "declarations.yaml",
		"unit: probe-words\nimports: [/opt/agent-core/tools/units/"+stage.words+"]\ntools: []\n")
	writeLoadFixture(t, root, "rest.yaml", `unit: probe-rest
expand:
  - fragment: /opt/providers/`+stage.fragment+`
    args: {`+stage.args+`}
rest: {version: v1}
`)
	writeLoadFixture(t, root, "profile.yaml", `name: provider-stage
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
rest_definitions: [rest.yaml]
libraries: {providers: `+library+`}
`)
	return filepath.Join(root, "profile.yaml")
}

func TestEmbedStageFragmentsSpliceWithEachLibrary(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetInstallRoot("") })
	core := agentCoreRoot(t)
	corepath.SetInstallRoot(core)
	cases := map[string][]string{
		"ollama":        {"embed-query", "embed-document"},
		"cohere":        {"embed-query", "embed-document", "rerank"},
		"openai-shaped": {"embed-query", "embed-document"},
	}
	for provider, stages := range cases {
		library := filepath.Join(core, "tools", "providers", provider)
		if provider == "openai-shaped" {
			library = filepath.Join(core, "testdata", "providers", provider)
		}
		for _, name := range stages {
			stage := providerStages[name]
			closure, err := LoadClosure(writeProviderStageProfile(t, stage, library), Options{CoreRoot: core})
			require.NoError(t, err, "%s stage with the %s library", name, provider)

			routed := map[string]string{}
			for _, transition := range closure.Machine.Transitions {
				if transition.Next == "Failed" {
					routed[transition.Signal] = transition.State
				}
			}
			for _, failure := range []string{"ProviderUnauthorized", "ProviderThrottled", "ProviderUnavailable", "CommandError"} {
				require.Contains(t, routed, failure, "%s/%s routes %s to Failed", provider, name, failure)
			}
			require.Len(t, closure.Selected, len(stage.selection))
		}
	}
}

func TestProviderLibraryMissingFragmentNamesPathAndDirectory(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetInstallRoot("") })
	core := agentCoreRoot(t)
	corepath.SetInstallRoot(core)
	ollama := filepath.Join(core, "tools", "providers", "ollama")

	_, err := LoadClosure(writeProviderStageProfile(t, providerStages["rerank"], ollama), Options{CoreRoot: core})

	require.ErrorContains(t, err, "rerank-fragment.yaml", "the missing path")
	require.ErrorContains(t, err, ollama, "the bound directory")
}
