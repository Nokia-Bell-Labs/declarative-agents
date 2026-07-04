// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadChartersValidatesAndSortsSuites(t *testing.T) {
	tmp := t.TempDir()
	second := writeCharter(t, tmp, "second.yaml", `
id: second-suite
checks:
  - id: docs-builtins
    kind: spec_corpus
`)
	first := writeCharter(t, tmp, "first.yaml", `
id: first-suite
target:
  root: docs
  include: ["**/*.md"]
checks:
  - id: no-internal-words
    kind: grep_check
    severity: warning
    include: ["papers/**/*.md"]
    patterns: ["cobbler"]
`)

	charters, err := LoadCharters([]string{second, first})

	require.NoError(t, err)
	require.Len(t, charters, 2)
	require.Equal(t, "first-suite", charters[0].ID)
	require.Equal(t, "docs", charters[0].Target.Root)
	require.Equal(t, "warning", charters[0].Checks[0].Severity)
	require.Equal(t, "second-suite", charters[1].ID)
	require.Equal(t, "error", charters[1].Checks[0].Severity)
}

func TestLoadChartersRejectsMissingFile(t *testing.T) {
	_, err := LoadCharters([]string{filepath.Join(t.TempDir(), "missing.yaml")})

	require.ErrorContains(t, err, "read charter")
	require.ErrorContains(t, err, "missing.yaml")
}

func TestParseCharterRejectsInvalidYAML(t *testing.T) {
	_, err := ParseCharter([]byte("id: ["))

	require.ErrorContains(t, err, "invalid YAML")
}

func TestParseCharterRejectsUnknownCheckKind(t *testing.T) {
	_, err := ParseCharter([]byte(`
id: bad-suite
checks:
  - id: mystery
    kind: invented_kind
`))

	require.ErrorContains(t, err, `unknown check kind "invented_kind"`)
}

func TestLoadChartersRejectsDuplicateSuiteIDs(t *testing.T) {
	tmp := t.TempDir()
	first := writeCharter(t, tmp, "a.yaml", `
id: duplicate-suite
checks:
  - id: a
    kind: spec_corpus
`)
	second := writeCharter(t, tmp, "b.yaml", `
id: duplicate-suite
checks:
  - id: b
    kind: spec_corpus
`)

	_, err := LoadCharters([]string{second, first})

	require.ErrorContains(t, err, `duplicate charter suite id "duplicate-suite"`)
	require.ErrorContains(t, err, "a.yaml")
	require.ErrorContains(t, err, "b.yaml")
}

func TestParseCharterRejectsDuplicateCheckIDs(t *testing.T) {
	_, err := ParseCharter([]byte(`
id: bad-suite
checks:
  - id: repeated
    kind: spec_corpus
  - id: repeated
    kind: grep_check
`))

	require.ErrorContains(t, err, `duplicate check id "repeated"`)
}

func TestParseCharterRejectsInvalidSeverity(t *testing.T) {
	_, err := ParseCharter([]byte(`
id: bad-suite
checks:
  - id: severity
    kind: spec_corpus
    severity: info
`))

	require.ErrorContains(t, err, `unknown severity "info"`)
}

func writeCharter(t *testing.T, dir, name, data string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
	return path
}
