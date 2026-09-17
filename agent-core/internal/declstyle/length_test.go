// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package declstyle

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// classLongDeclarationFile marks a declaration YAML file over
// maxDeclarationFileLines. One concept belongs in one file; a file that long is
// several concepts or a repeated shape, and the fix is a split or a fragment,
// not a longer baseline (GH-2111, execution constitution E11). The baseline
// seeds today's long files and only shrinks, as internal/gostyle does for Go
// files over 500 lines.
const classLongDeclarationFile = "long-declaration-file"

const maxDeclarationFileLines = 300

// longFileEntries returns each over-long file's entry with its physical line
// count, so a new entry can be reported with its length.
func longFileEntries(paths []string, repo string) (map[string]int, error) {
	entries := map[string]int{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		lines := physicalLines(data)
		if lines <= maxDeclarationFileLines {
			continue
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return nil, err
		}
		entries[fmt.Sprintf("%s:%s", classLongDeclarationFile, filepath.ToSlash(rel))] = lines
	}
	return entries, nil
}

// physicalLines counts lines the way an editor shows them: a final line
// without a trailing newline still counts.
func physicalLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

// describeNewEntry adds the line count to a new long-file entry, so the failure
// names the length it has to lose.
func describeNewEntry(entry string, lengths map[string]int) string {
	if lines, ok := lengths[entry]; ok {
		return fmt.Sprintf("%s (%d lines, limit %d)", entry, lines, maxDeclarationFileLines)
	}
	return entry
}

func TestLongDeclarationFileClassification(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, lines int) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("tools:\n"+strings.Repeat("  - word\n", lines-1)), 0o644))
	}
	write("agents/fits/declarations.yaml", 300)
	write("agents/long/declarations.yaml", 301)
	write("build/profiles/long.yaml", 900)

	paths, err := discoverLegacyDeclarationFiles([]string{root})
	require.NoError(t, err)
	entries, err := longFileEntries(paths, root)
	require.NoError(t, err)

	entry := classLongDeclarationFile + ":agents/long/declarations.yaml"
	require.Equal(t, map[string]int{entry: 301}, entries,
		"only the 301-line authored file is long; generated trees stay excluded")
	require.Equal(t, entry+" (301 lines, limit 300)", describeNewEntry(entry, entries))
}
