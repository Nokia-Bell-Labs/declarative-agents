// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

func yamlFilesInDir(dir string) []string {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	return matches
}

func resolveProfileAssetsRoot(rootDir string) string {
	for _, candidate := range profileRootCandidates(rootDir) {
		if root := normalizeProfileRoot(candidate); root != "" {
			return root
		}
	}
	return filepath.Join(rootDir, AgentsDir)
}

func profileRootCandidates(rootDir string) []string {
	return []string{
		filepath.Join(filepath.Dir(rootDir), "agent-profiles"),
		filepath.Join(rootDir, "agent-profiles"),
	}
}

func normalizeProfileRoot(candidate string) string {
	for _, root := range []string{candidate, filepath.Join(candidate, AgentsDir)} {
		if profileExists(root, "generator") || profileExists(root, "jurist") {
			return root
		}
	}
	return ""
}

func profileExists(root, rel string) bool {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel), "profile.yaml"))
	return err == nil && !info.IsDir()
}

func declarationFilesFromProfile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var profile struct {
		ToolDeclarations []string `yaml:"tool_declarations"`
		ToolConfigDirs   []string `yaml:"tool_config_dirs"`
	}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil
	}
	base := filepath.Dir(path)
	var files []string
	for _, dir := range profile.ToolConfigDirs {
		files = append(files, yamlFilesInDir(resolveProfilePath(base, dir))...)
	}
	for _, decl := range profile.ToolDeclarations {
		files = append(files, resolveProfilePath(base, decl))
	}
	return files
}

func resolveProfilePath(base, p string) string {
	if mapped := MapInstalledCorePath(p); mapped != "" {
		return mapped
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

