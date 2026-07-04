// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ExecuteGrepChecks runs grep_check charter checks over targetDir.
func ExecuteGrepChecks(targetDir string, charters []Charter) ([]Finding, error) {
	var findings []Finding
	for _, charter := range charters {
		root, rootRel := charterRoot(targetDir, charter.Target.Root)
		files, err := charterFiles(root, rootRel, charter.Target.Include, charter.Target.Exclude)
		if err != nil {
			return nil, fmt.Errorf("charter %q: %w", charter.ID, err)
		}
		for _, check := range charter.Checks {
			if check.Kind != "grep_check" {
				continue
			}
			checkFindings, err := executeGrepCheck(charter, check, root, rootRel, files)
			if err != nil {
				return nil, err
			}
			findings = append(findings, checkFindings...)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		return findingLess(findings[i], findings[j])
	})
	return findings, nil
}

func executeGrepCheck(charter Charter, check CharterCheck, root, rootRel string, baseFiles []charterFile) ([]Finding, error) {
	if len(check.Patterns) == 0 {
		return nil, fmt.Errorf("charter %q check %q: grep_check requires patterns", charter.ID, check.ID)
	}
	mode := check.Mode
	if mode == "" {
		mode = "match"
	}
	if mode != "match" && mode != "missing" {
		return nil, fmt.Errorf("charter %q check %q: unknown grep_check mode %q", charter.ID, check.ID, check.Mode)
	}
	compiled, err := compileGrepPatterns(check)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
	}

	files, err := narrowCharterFiles(root, rootRel, baseFiles, check.Include, check.Exclude)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
	}

	var findings []Finding
	var matched bool
	for _, file := range files {
		data, err := os.ReadFile(file.abs)
		if err != nil {
			return nil, fmt.Errorf("read target file %s: %w", file.display, err)
		}
		lines := strings.Split(string(data), "\n")
		for idx, line := range lines {
			if idx == len(lines)-1 && line == "" {
				continue
			}
			for _, pattern := range compiled {
				if !pattern.matches(line) {
					continue
				}
				matched = true
				if mode == "match" {
					findings = append(findings, grepFinding(charter, check, file.display, idx+1, pattern.raw))
				}
			}
		}
	}
	if mode == "missing" && !matched {
		findings = append(findings, grepFinding(charter, check, "", 0, strings.Join(check.Patterns, ", ")))
	}
	return findings, nil
}

type charterFile struct {
	abs     string
	rel     string
	display string
}

func charterRoot(targetDir, root string) (string, string) {
	if root == "" {
		root = "."
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root), "."
	}
	return filepath.Join(targetDir, root), filepath.ToSlash(filepath.Clean(root))
}

func charterFiles(root, rootRel string, include, exclude []string) ([]charterFile, error) {
	var files []charterFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		display := displayCharterPath(rootRel, rel)
		if !includedByGlob(rel, include) || excludedByGlob(rel, exclude) {
			return nil
		}
		files = append(files, charterFile{abs: path, rel: rel, display: display})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover target files: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].display < files[j].display
	})
	return files, nil
}

func narrowCharterFiles(root, rootRel string, baseFiles []charterFile, include, exclude []string) ([]charterFile, error) {
	if len(include) == 0 && len(exclude) == 0 {
		return append([]charterFile(nil), baseFiles...), nil
	}
	return charterFiles(root, rootRel, include, exclude)
}

func displayCharterPath(rootRel, rel string) string {
	if rootRel == "." || rootRel == "" {
		return rel
	}
	return filepath.ToSlash(filepath.Join(rootRel, rel))
}

func includedByGlob(path string, include []string) bool {
	if len(include) == 0 {
		return true
	}
	for _, pattern := range include {
		if matchCharterGlob(pattern, path) {
			return true
		}
	}
	return false
}

func excludedByGlob(path string, exclude []string) bool {
	for _, pattern := range exclude {
		if matchCharterGlob(pattern, path) {
			return true
		}
	}
	return false
}

func matchCharterGlob(pattern, path string) bool {
	pattern = filepath.ToSlash(filepath.Clean(strings.TrimSpace(pattern)))
	path = filepath.ToSlash(filepath.Clean(path))
	if pattern == "." || pattern == "" {
		return false
	}
	return matchGlobParts(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func matchGlobParts(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		if matchGlobParts(pattern[1:], path) {
			return true
		}
		return len(path) > 0 && matchGlobParts(pattern, path[1:])
	}
	if len(path) == 0 {
		return false
	}
	matched, err := filepath.Match(pattern[0], path[0])
	if err != nil || !matched {
		return false
	}
	return matchGlobParts(pattern[1:], path[1:])
}

type grepPattern struct {
	raw string
	re  *regexp.Regexp
}

func compileGrepPatterns(check CharterCheck) ([]grepPattern, error) {
	patterns := make([]grepPattern, 0, len(check.Patterns))
	for _, raw := range check.Patterns {
		if !check.Regex {
			patterns = append(patterns, grepPattern{raw: raw})
			continue
		}
		re, err := regexp.Compile(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", raw, err)
		}
		patterns = append(patterns, grepPattern{raw: raw, re: re})
	}
	return patterns, nil
}

func (p grepPattern) matches(line string) bool {
	if p.re != nil {
		return p.re.MatchString(line)
	}
	return strings.Contains(line, p.raw)
}

func grepFinding(charter Charter, check CharterCheck, file string, line int, pattern string) Finding {
	message := check.Message
	if message == "" {
		message = fmt.Sprintf("pattern %q matched", pattern)
	}
	if check.Mode == "missing" && file == "" {
		message = fmt.Sprintf("pattern %q not found", pattern)
		if check.Message != "" {
			message = check.Message
		}
	}
	return Finding{
		Check:   check.ID,
		Level:   check.Severity,
		Message: message,
		SuiteID: charter.ID,
		CheckID: check.ID,
		Kind:    check.Kind,
		File:    file,
		Line:    line,
	}
}
