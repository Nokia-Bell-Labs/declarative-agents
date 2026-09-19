// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package declstyle

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// legacyProviderLine is an invoke_llm config naming a provider value the
// shipped dialects replaced (srd058 R3.2).
var legacyProviderLine = regexp.MustCompile(`(?m)^\s*provider:\s*"?(ollama|cohere)"?\s*$`)

// legacyProviderFixtures exercise the legacy mapping on purpose: they load
// declarations with no profile, which is the case the mapping serves.
var legacyProviderFixtures = map[string]bool{
	"agent-core/internal/tools/rest/testdata/ollama_profile/ollama-llm.yaml": true,
	"applications/catalog/testdata/conformance/rest/ollama-llm.yaml":         true,
}

// legacyProviderDeclarations lists declaration files and templates, under the
// monorepo's agent-core and applications trees, whose invoke_llm config still
// names a legacy provider. A template counts: the #2241 sweep read only
// *.yaml and missed a *.tmpl fixture a harness renders (GH-2257).
func legacyProviderDeclarations(repo string) ([]string, error) {
	var found []string
	for _, tree := range []string{"agent-core", "applications"} {
		err := filepath.WalkDir(filepath.Join(repo, tree), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "node_modules" || name == "build" || name == "dist" || name == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".tmpl") {
				return nil
			}
			rel, _ := filepath.Rel(repo, path)
			rel = filepath.ToSlash(rel)
			if legacyProviderFixtures[rel] || strings.Contains(rel, "/docs/") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			if strings.Contains(text, "init: invoke_llm") && legacyProviderLine.MatchString(text) {
				found = append(found, rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(found)
	return found, nil
}

func TestNoFixtureNamesALegacyProvider(t *testing.T) {
	found, err := legacyProviderDeclarations(filepath.Dir(moduleRoot(t)))

	require.NoError(t, err)
	require.Empty(t, found, "invoke_llm declarations name a dialect under the providers root (srd058 R1.1, R3.2)")
}
