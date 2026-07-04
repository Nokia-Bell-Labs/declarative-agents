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

func TestParseCharterRejectsMissingKindSpecificConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "grep patterns",
			body: `
id: bad-suite
checks:
  - id: no-words
    kind: grep_check
`,
			want: "grep_check requires patterns",
		},
		{
			name: "ref references",
			body: `
id: bad-suite
checks:
  - id: refs
    kind: ref_check
    extract:
      regex: "@([A-Za-z0-9:_-]+)"
`,
			want: "ref_check requires references",
		},
		{
			name: "ref extract regex",
			body: `
id: bad-suite
checks:
  - id: refs
    kind: ref_check
    references:
      inline: ["known"]
`,
			want: "ref_check requires extract.regex",
		},
		{
			name: "consistency source yaml path",
			body: `
id: bad-suite
checks:
  - id: consistent
    kind: consistency_check
    source:
      file: manifest.yaml
    rule: required_path_exists
`,
			want: "consistency_check requires source.yaml_path",
		},
		{
			name: "consistency rule",
			body: `
id: bad-suite
checks:
  - id: consistent
    kind: consistency_check
    source:
      yaml_path: "$.artifacts[*].path"
`,
			want: "consistency_check requires rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCharter([]byte(tt.body))

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestParseCharterRejectsUnknownSpecCorpusSubset(t *testing.T) {
	_, err := ParseCharter([]byte(`
id: bad-suite
checks:
  - id: spec-corpus
    kind: spec_corpus
    checks:
      - invented-check
`))

	require.ErrorContains(t, err, `unknown spec_corpus check "invented-check"`)
}

func writeCharter(t *testing.T, dir, name, data string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
	return path
}
