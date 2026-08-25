// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

func TestReconstructProgramResourcesVerifiesFiltersAndMerges(t *testing.T) {
	ref := core.ProgramRef{Profile: "/profiles/origin.yaml", Digest: "digest"}
	execution := core.Execution{
		{CommandName: "origin_write", Receipt: "opaque"},
		{CommandName: "complete_without_receipt"},
	}
	current := ProgramResources{
		Definitions: []catalog.ToolDef{{Name: "checkpoint_rollback"}},
		RestDefinitions: toolrest.Collection{
			Auth: map[string]restdef.AuthProfile{
				"shared": {Type: "basic"},
				"local":  {Type: "bearer"},
			},
		},
	}

	got, err := ReconstructProgramResources(ref, execution, current,
		func(core.ProgramRef) (ReferencedProgram, error) {
			return ReferencedProgram{
				Ref: ref,
				ProgramResources: ProgramResources{
					Definitions: []catalog.ToolDef{
						{Name: "origin_write", Binary: "true"},
						{Name: "unused", Binary: "false"},
					},
					RestDefinitions: toolrest.Collection{
						Auth: map[string]restdef.AuthProfile{
							"shared": {Type: "token"},
							"origin": {Type: "api_key"},
						},
					},
				},
			}, nil
		})
	require.NoError(t, err)
	require.Equal(t, []string{"origin_write", "checkpoint_rollback"}, definitionNames(got.Definitions))
	require.Equal(t, "basic", got.RestDefinitions.Auth["shared"].Type)
	require.Equal(t, "api_key", got.RestDefinitions.Auth["origin"].Type)
	require.Equal(t, "bearer", got.RestDefinitions.Auth["local"].Type)
}

func TestReconstructProgramResourcesRejectsIncompatiblePrograms(t *testing.T) {
	current := ProgramResources{}
	tests := []struct {
		name      string
		ref       core.ProgramRef
		execution core.Execution
		load      ProgramLoader
		want      string
	}{
		{
			name: "missing identity",
			want: "no declarative program reference",
		},
		{
			name: "digest drift",
			ref:  core.ProgramRef{Profile: "/profiles/origin.yaml", Digest: "expected"},
			load: func(core.ProgramRef) (ReferencedProgram, error) {
				return ReferencedProgram{Ref: core.ProgramRef{Digest: "changed"}}, nil
			},
			want: "target program /profiles/origin.yaml changed",
		},
		{
			name:      "missing receipt builder",
			ref:       core.ProgramRef{Profile: "/profiles/origin.yaml", Digest: "digest"},
			execution: core.Execution{{CommandName: "missing", Receipt: "opaque"}},
			load: func(ref core.ProgramRef) (ReferencedProgram, error) {
				return ReferencedProgram{Ref: ref}, nil
			},
			want: `does not declare receipt-bearing command "missing"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ReconstructProgramResources(test.ref, test.execution, current, test.load)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func definitionNames(defs []catalog.ToolDef) []string {
	names := make([]string, len(defs))
	for i, def := range defs {
		names[i] = def.Name
	}
	return names
}
