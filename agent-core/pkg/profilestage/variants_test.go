// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profilestage_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profilestage"
)

// A machine that selects its stage by environment (srd052 R4.3) stages every
// variant, because the binding is chosen where the profile runs.
func variantMachineSource(t *testing.T, fragment string, variants ...string) string {
	t.Helper()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/rag/machine.yaml",
		"name: rag\ninstantiate:\n- fragment: "+fragment+"\n  args: {prefix: Rerank}\nstates: []\n")
	for _, variant := range variants {
		writeDeclaration(t, source, "agents/stages/"+variant,
			"unit: stage\nparams:\n- {name: prefix, type: string}\nstage: {transitions: []}\n")
	}
	return source
}

func TestStageCarriesEveryVariantOfAnEnvironmentSelectedStage(t *testing.T) {
	t.Parallel()
	source := variantMachineSource(t, "../stages/rerank-${PROVIDER:-none}.yaml",
		"rerank-none.yaml", "rerank-cohere.yaml", "compose.yaml")
	root := t.TempDir()

	require.NoError(t, profilestage.Stage(root, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "rag"),
		Destination: filepath.Join(root, "agents", "rag"),
	}))

	require.FileExists(t, filepath.Join(root, "agents", "stages", "rerank-none.yaml"))
	require.FileExists(t, filepath.Join(root, "agents", "stages", "rerank-cohere.yaml"))
	require.NoFileExists(t, filepath.Join(root, "agents", "stages", "compose.yaml"),
		"only files the templated path can name are staged")
	imported, err := profilestage.Imported(filepath.Join(source, "agents", "rag"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		filepath.Join(source, "agents", "stages", "rerank-cohere.yaml"),
		filepath.Join(source, "agents", "stages", "rerank-none.yaml"),
	}, imported)
}

func TestStageRefusesAVariantPathItCannotStage(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		fragment string
		variants []string
		want     string
	}{
		"default missing": {"../stages/rerank-${PROVIDER:-none}.yaml", []string{"rerank-cohere.yaml"},
			"default variant ../stages/rerank-none.yaml is missing"},
		"no default": {"../stages/rerank-${PROVIDER}.yaml", []string{"rerank-none.yaml"},
			"needs the form ${NAME:-default}"},
	} {
		t.Run(name, func(t *testing.T) {
			source := variantMachineSource(t, test.fragment, test.variants...)
			root := t.TempDir()
			err := profilestage.Stage(root, profilestage.Tree{
				Source:      filepath.Join(source, "agents", "rag"),
				Destination: filepath.Join(root, "agents", "rag"),
			})
			require.ErrorContains(t, err, test.want)
			_, err = profilestage.Imported(filepath.Join(source, "agents", "rag"))
			require.ErrorContains(t, err, test.want)
		})
	}
}
