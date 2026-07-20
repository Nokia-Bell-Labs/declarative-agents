// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// profileDir names a discovered agent profile directory. Name is the logical
// agent key, the leaf directory name in both the top-level and nested cases, so
// it matches the machine spec name that machine-name consistency checks compare
// against. Dir is the absolute directory that holds machine.yaml and the rest of
// the profile assets.
type profileDir struct {
	Name string
	Dir  string
}

// collectProfileDirs enumerates agent profile directories under profilesPath. A
// directory that holds machine.yaml or profile.yaml is a profile. A directory
// that holds neither is treated as a family, and its immediate subdirectories
// are scanned one level deeper, so profiles grouped under a family
// (knowledge-manager/corpus-reader) are discovered rather than silently skipped.
// Recursion stops at one level, and profiles are keyed by their leaf directory
// name.
func collectProfileDirs(profilesPath string) []profileDir {
	entries, err := os.ReadDir(profilesPath)
	if err != nil {
		return nil
	}
	var dirs []profileDir
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		top := filepath.Join(profilesPath, entry.Name())
		if isProfileDir(top) {
			dirs = append(dirs, profileDir{Name: entry.Name(), Dir: top})
			continue
		}
		subs, err := os.ReadDir(top)
		if err != nil {
			continue
		}
		for _, sub := range subs {
			if !sub.IsDir() {
				continue
			}
			subDir := filepath.Join(top, sub.Name())
			if isProfileDir(subDir) {
				dirs = append(dirs, profileDir{Name: sub.Name(), Dir: subDir})
			}
		}
	}
	return dirs
}

// isProfileDir reports whether dir is an agent profile directory, identified by
// a machine.yaml or a profile.yaml. Machine discovery narrows this further to
// directories that actually carry machine.yaml.
func isProfileDir(dir string) bool {
	return regularFileExists(filepath.Join(dir, "machine.yaml")) ||
		regularFileExists(filepath.Join(dir, "profile.yaml"))
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

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
		if profileExists(root, "executor") || profileExists(root, "jurist") {
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
