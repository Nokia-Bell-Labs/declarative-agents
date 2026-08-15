// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestPhaseScopedChapterExamplesLoadAndDeriveAvailability(t *testing.T) {
	chapterPath := filepath.Join("..", "..", "..", "..",
		"design-patterns", "05-phase-scoped-toolset.md")
	chapter, err := os.ReadFile(chapterPath)
	require.NoError(t, err)

	dir := t.TempDir()
	machinePath := writePhaseExample(t, dir, "machine.yaml", chapter,
		"phase-scoped-machine-example")
	toolsPath := writePhaseExample(t, dir, "tools.yaml", chapter,
		"phase-scoped-tools-example")

	machine, err := core.LoadMachineSpec(machinePath)
	require.NoError(t, err, "chapter machine example must load")
	defs, err := LoadToolDefs(toolsPath)
	require.NoError(t, err, "chapter ToolDef example must load")
	require.NoError(t, ValidateToolEmits(machine, defs),
		"chapter examples must have routable emitted signals")

	derived := ApplyDynamicToolPhases(machine, defs)
	byName := map[string]ToolDef{}
	for _, def := range derived {
		byName[def.Name] = def
	}
	writeDef := byName["write"]
	write := writeDef.ToToolSpec()
	require.True(t, write.AvailableIn("Composing"))
	require.False(t, write.AvailableIn("Parsing"))
	require.True(t, write.PhaseScoped)

	webSearchDef := byName["web_search"]
	webSearch := webSearchDef.ToToolSpec()
	require.False(t, webSearch.AvailableIn("Composing"),
		"explicit Reviewing phase may narrow but not widen derived availability")

	parseResponseDef := byName["parse_response"]
	parseResponse := parseResponseDef.ToToolSpec()
	require.Equal(t, core.Internal, parseResponse.Visibility)
}

func writePhaseExample(
	t *testing.T,
	dir, name string,
	chapter []byte,
	marker string,
) string {
	t.Helper()
	pattern := regexp.MustCompile("(?s)```yaml\\n# " +
		regexp.QuoteMeta(marker) + "\\n(.*?)```")
	match := pattern.FindSubmatch(chapter)
	require.Len(t, match, 2, "missing YAML example %s", marker)
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, match[1], 0o644))
	return path
}
