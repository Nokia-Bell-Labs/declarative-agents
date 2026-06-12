// Copyright (c) 2026 Nokia. All rights reserved.

package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Assemble
// ---------------------------------------------------------------------------

func TestAssemble_AllSections(t *testing.T) {
	t.Parallel()
	p := Prompt{
		Role:         "role text",
		Task:         "task text",
		Constraints:  "constraint text",
		OutputFormat: "format text",
	}
	got := p.Assemble()
	require.Equal(t, "role text\n\ntask text\n\nconstraint text\n\nformat text", got)
}

func TestAssemble_EmptySectionsOmitted(t *testing.T) {
	t.Parallel()
	p := Prompt{Task: "do the thing"}
	got := p.Assemble()
	require.Equal(t, "do the thing", got)
}

func TestAssemble_WhitespaceOnlySectionsOmitted(t *testing.T) {
	t.Parallel()
	p := Prompt{Role: "  \n ", Task: "task", Constraints: "\t"}
	got := p.Assemble()
	require.Equal(t, "task", got)
}

func TestAssemble_Deterministic(t *testing.T) {
	t.Parallel()
	p := Prompt{Role: "r", Task: "t", Constraints: "c", OutputFormat: "o"}
	a := p.Assemble()
	b := p.Assemble()
	require.Equal(t, a, b)
}

func TestAssemble_PartialSections(t *testing.T) {
	t.Parallel()
	p := Prompt{Role: "r", Task: "t"}
	got := p.Assemble()
	require.Equal(t, "r\n\nt", got)
}

// ---------------------------------------------------------------------------
// SectionCount
// ---------------------------------------------------------------------------

func TestSectionCount(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, Prompt{}.SectionCount())
	require.Equal(t, 1, Prompt{Task: "x"}.SectionCount())
	require.Equal(t, 4, Prompt{Role: "a", Task: "b", Constraints: "c", OutputFormat: "d"}.SectionCount())
	require.Equal(t, 1, Prompt{Role: "  ", Task: "t", Constraints: "\n"}.SectionCount())
}

// ---------------------------------------------------------------------------
// LoadPrompt — structured mode
// ---------------------------------------------------------------------------

func writePromptFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadPrompt_StructuredFull(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `
task: "implement feature X"
role: "custom role"
constraints: "custom constraints"
output_format: "custom format"
`)
	lr, err := LoadPrompt(path)
	require.NoError(t, err)
	require.Equal(t, "structured", lr.Mode)
	require.Equal(t, "implement feature X", lr.Prompt.Task)
	require.Equal(t, "custom role", lr.Prompt.Role)
	require.Equal(t, "custom constraints", lr.Prompt.Constraints)
	require.Equal(t, "custom format", lr.Prompt.OutputFormat)
	require.Equal(t, 4, lr.Sections)
}

func TestLoadPrompt_StructuredDefaults(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `task: "do something"`)
	lr, err := LoadPrompt(path)
	require.NoError(t, err)
	require.Equal(t, "structured", lr.Mode)
	require.Equal(t, "do something", lr.Prompt.Task)
	require.Equal(t, DefaultRole, lr.Prompt.Role)
	require.Equal(t, DefaultConstraints, lr.Prompt.Constraints)
	require.Equal(t, DefaultOutputFormat, lr.Prompt.OutputFormat)
}

func TestLoadPrompt_StructuredPartialOverride(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `
task: "my task"
role: "my role"
`)
	lr, err := LoadPrompt(path)
	require.NoError(t, err)
	require.Equal(t, "my task", lr.Prompt.Task)
	require.Equal(t, "my role", lr.Prompt.Role)
	require.Equal(t, DefaultConstraints, lr.Prompt.Constraints)
	require.Equal(t, DefaultOutputFormat, lr.Prompt.OutputFormat)
}

// ---------------------------------------------------------------------------
// LoadPrompt — simple mode
// ---------------------------------------------------------------------------

func TestLoadPrompt_Simple(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `prompt: "build a CLI tool"`)
	lr, err := LoadPrompt(path)
	require.NoError(t, err)
	require.Equal(t, "simple", lr.Mode)
	require.Equal(t, "build a CLI tool", lr.Prompt.Task)
	require.Equal(t, DefaultRole, lr.Prompt.Role)
}

// ---------------------------------------------------------------------------
// LoadPrompt — error cases
// ---------------------------------------------------------------------------

func TestLoadPrompt_BothFieldsError(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `
task: "a"
prompt: "b"
`)
	_, err := LoadPrompt(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "both")
}

func TestLoadPrompt_MissingFields(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `role: "something"`)
	_, err := LoadPrompt(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
}

func TestLoadPrompt_EmptyTask(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `task: ""`)
	_, err := LoadPrompt(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

func TestLoadPrompt_WhitespaceOnlyTask(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `task: "   "`)
	_, err := LoadPrompt(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

func TestLoadPrompt_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadPrompt("/nonexistent/prompt.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read prompt file")
}

func TestLoadPrompt_InvalidYAML(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `{{{not valid yaml`)
	_, err := LoadPrompt(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse prompt YAML")
}

// ---------------------------------------------------------------------------
// LoadPromptFromString
// ---------------------------------------------------------------------------

func TestLoadPromptFromString_Success(t *testing.T) {
	t.Parallel()
	lr, err := LoadPromptFromString("plan the migration")
	require.NoError(t, err)
	require.Equal(t, "string", lr.Mode)
	require.Equal(t, "plan the migration", lr.Prompt.Task)
	require.Equal(t, DefaultRole, lr.Prompt.Role)
	require.Equal(t, DefaultConstraints, lr.Prompt.Constraints)
	require.Equal(t, DefaultOutputFormat, lr.Prompt.OutputFormat)
	require.Equal(t, 4, lr.Sections)
}

func TestLoadPromptFromString_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	lr, err := LoadPromptFromString("  my task  \n")
	require.NoError(t, err)
	require.Equal(t, "my task", lr.Prompt.Task)
}

func TestLoadPromptFromString_EmptyError(t *testing.T) {
	t.Parallel()
	_, err := LoadPromptFromString("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

func TestLoadPromptFromString_WhitespaceOnlyError(t *testing.T) {
	t.Parallel()
	_, err := LoadPromptFromString("   \n\t  ")
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

// ---------------------------------------------------------------------------
// LoadResult metadata
// ---------------------------------------------------------------------------

func TestLoadResult_Metadata(t *testing.T) {
	t.Parallel()
	yaml := `task: "implement feature"` + "\n"
	path := writePromptFile(t, yaml)
	lr, err := LoadPrompt(path)
	require.NoError(t, err)
	require.Equal(t, len(yaml), lr.FileSize)
	require.Equal(t, len("implement feature"), lr.TaskLen)
	require.Equal(t, 4, lr.Sections) // defaults fill all four
}

// ---------------------------------------------------------------------------
// Default constants
// ---------------------------------------------------------------------------

func TestDefaults_NonEmpty(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, DefaultRole)
	require.NotEmpty(t, DefaultConstraints)
	require.NotEmpty(t, DefaultOutputFormat)
}

func TestDefaults_AppliedInSimpleMode(t *testing.T) {
	t.Parallel()
	lr, err := LoadPromptFromString("task")
	require.NoError(t, err)
	require.Equal(t, DefaultRole, lr.Prompt.Role)
	require.Equal(t, DefaultConstraints, lr.Prompt.Constraints)
	require.Equal(t, DefaultOutputFormat, lr.Prompt.OutputFormat)
}

// ---------------------------------------------------------------------------
// Unknown fields are ignored
// ---------------------------------------------------------------------------

func TestLoadPrompt_UnknownFieldsIgnored(t *testing.T) {
	t.Parallel()
	path := writePromptFile(t, `
task: "do work"
unknown_field: "should be ignored"
another: 42
`)
	lr, err := LoadPrompt(path)
	require.NoError(t, err)
	require.Equal(t, "do work", lr.Prompt.Task)
}
