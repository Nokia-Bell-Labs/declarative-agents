// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package registry

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandardFactoryCatalogSelectsEveryRegisteredInit(t *testing.T) {
	t.Parallel()

	deps := testFactoryDeps()
	entries := StandardFactoryCatalog(deps)
	require.Len(t, entries, 13)

	for _, entry := range entries {
		require.ElementsMatch(t, []string{
			fmt.Sprintf("%s_first", entry.Name),
			fmt.Sprintf("%s_second", entry.Name),
		}, entry.Inits)

		for _, initName := range entry.Inits {
			t.Run(entry.Name+"/"+initName, func(t *testing.T) {
				br := NewBuiltinRegistry()
				RegisterStandardBuiltinFactories(br, map[string]bool{initName: true}, deps)

				require.ElementsMatch(t, entry.Inits, br.Names())
			})
		}
	}

	br := NewBuiltinRegistry()
	RegisterStandardBuiltinFactories(br, map[string]bool{"not_registered": true}, deps)
	require.Empty(t, br.Names())
}

func TestRegisterStandardBuiltinFactoriesProfilesSelectOnlyMatchingFamily(t *testing.T) {
	t.Parallel()

	deps := StandardFactoryDeps{
		RegisterPlanning: registrarForInits("load_graph"),
		RegisterSpecValidation: registrarForInits(
			"load_corpus",
			"format_report",
		),
		RegisterOTLP: registrarForInits(
			"spool_spans",
			"spool_get_metric",
		),
	}

	tests := []struct {
		name     string
		selected string
		want     []string
	}{
		{
			name:     "format report only",
			selected: "format_report",
			want:     []string{"load_corpus", "format_report"},
		},
		{
			name:     "spool get metric only",
			selected: "spool_get_metric",
			want:     []string{"spool_spans", "spool_get_metric"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			br := NewBuiltinRegistry()
			RegisterStandardBuiltinFactories(br, map[string]bool{tc.selected: true}, deps)

			require.ElementsMatch(t, tc.want, br.Names())
		})
	}
}

func TestStandardFactoryCatalogHandlesNilHooks(t *testing.T) {
	t.Parallel()

	entries := StandardFactoryCatalog(StandardFactoryDeps{})
	require.Len(t, entries, 13)
	for _, entry := range entries {
		require.Empty(t, entry.Inits)
		require.NotPanics(t, func() {
			entry.Register(NewBuiltinRegistry())
		})
	}

	br := NewBuiltinRegistry()
	require.NotPanics(t, func() {
		RegisterStandardBuiltinFactories(br, map[string]bool{"format_report": true}, StandardFactoryDeps{})
	})
	require.Empty(t, br.Names())
}

func testFactoryDeps() StandardFactoryDeps {
	return StandardFactoryDeps{
		RegisterFilesystem:     registrarForFamily("filesystem"),
		RegisterLLM:            registrarForFamily("llm"),
		RegisterLifecycle:      registrarForFamily("lifecycle"),
		RegisterControl:        registrarForFamily("control"),
		RegisterPlanning:       registrarForFamily("planning"),
		RegisterEvaluation:     registrarForFamily("evaluation"),
		RegisterSpecValidation: registrarForFamily("spec_validation"),
		RegisterREST:           registrarForFamily("rest"),
		RegisterDolt:           registrarForFamily("dolt"),
		RegisterCompose:        registrarForFamily("compose"),
		RegisterService:        registrarForFamily("service"),
		RegisterOTLP:           registrarForFamily("otlp"),
		RegisterPipeline:       registrarForFamily("pipeline"),
	}
}

func registrarForFamily(family string) FactoryRegistrar {
	return registrarForInits(
		fmt.Sprintf("%s_first", family),
		fmt.Sprintf("%s_second", family),
	)
}

func registrarForInits(inits ...string) FactoryRegistrar {
	return func(br *BuiltinRegistry) {
		for _, initName := range inits {
			br.Register(initName, nil)
		}
	}
}
