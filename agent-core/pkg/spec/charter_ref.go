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

// ExecuteRefChecks runs ref_check charter checks over targetDir.
func ExecuteRefChecks(targetDir string, charters []Charter) ([]Finding, error) {
	var findings []Finding
	for _, charter := range charters {
		root, rootRel := charterRoot(targetDir, charter.Target.Root)
		files, err := charterFiles(root, rootRel, charter.Target.Include, charter.Target.Exclude)
		if err != nil {
			return nil, fmt.Errorf("charter %q: %w", charter.ID, err)
		}
		for _, check := range charter.Checks {
			if check.Kind != "ref_check" {
				continue
			}
			checkFindings, err := executeRefCheck(targetDir, charter, check, root, rootRel, files)
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

func executeRefCheck(targetDir string, charter Charter, check CharterCheck, root, rootRel string, baseFiles []charterFile) ([]Finding, error) {
	extractor, err := refExtractor(check)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
	}
	allowed, err := refInventory(targetDir, root, check)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
	}
	files, err := narrowCharterFiles(root, rootRel, baseFiles, check.Include, check.Exclude)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
	}

	var findings []Finding
	var extracted int
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
			for _, ref := range extractor.extract(line) {
				extracted++
				if allowed[ref] {
					continue
				}
				findings = append(findings, refFinding(charter, check, file.display, idx+1, ref))
			}
		}
	}
	if extracted == 0 && !check.AllowMissing {
		findings = append(findings, refFinding(charter, check, "", 0, ""))
	}
	return findings, nil
}

type refRegexExtractor struct {
	re    *regexp.Regexp
	group int
}

func refExtractor(check CharterCheck) (refRegexExtractor, error) {
	raw, ok := stringMapValue(check.Extract, "regex")
	if !ok || raw == "" {
		return refRegexExtractor{}, fmt.Errorf("ref_check requires extract.regex")
	}
	group := 1
	if configured, ok := intMapValue(check.Extract, "group"); ok {
		group = configured
	}
	re, err := regexp.Compile(raw)
	if err != nil {
		return refRegexExtractor{}, fmt.Errorf("invalid extract regex %q: %w", raw, err)
	}
	if group < 0 || group > re.NumSubexp() {
		return refRegexExtractor{}, fmt.Errorf("extract group %d out of range for regex %q", group, raw)
	}
	return refRegexExtractor{re: re, group: group}, nil
}

func (e refRegexExtractor) extract(line string) []string {
	matches := e.re.FindAllStringSubmatch(line, -1)
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		if e.group >= len(match) {
			continue
		}
		ref := strings.TrimSpace(match[e.group])
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func refInventory(targetDir, root string, check CharterCheck) (map[string]bool, error) {
	if len(check.Refs) == 0 {
		return nil, fmt.Errorf("ref_check requires references")
	}
	allowed := make(map[string]bool)
	for _, key := range []string{"values", "keys", "inline"} {
		for _, value := range stringSliceMapValue(check.Refs, key) {
			allowed[value] = true
		}
	}
	if path, ok := stringMapValue(check.Refs, "file"); ok && path != "" {
		values, err := refsFromFile(resolveCharterPath(targetDir, root, path), check.Refs)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			allowed[value] = true
		}
	}
	if dir, ok := stringMapValue(check.Refs, "directory"); ok && dir != "" {
		values, err := refsFromDirectory(resolveCharterPath(targetDir, root, dir))
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			allowed[value] = true
		}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("ref_check references produced no allowed values")
	}
	return allowed, nil
}

func refsFromFile(path string, refs map[string]any) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read references file %s: %w", path, err)
	}
	if format, _ := stringMapValue(refs, "format"); format == "bibtex_keys" {
		return bibtexKeys(string(data)), nil
	}
	var values []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		values = append(values, line)
	}
	sort.Strings(values)
	return values, nil
}

func refsFromDirectory(root string) ([]string, error) {
	var values []string
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
		values = append(values, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read references directory %s: %w", root, err)
	}
	sort.Strings(values)
	return values, nil
}

func bibtexKeys(data string) []string {
	re := regexp.MustCompile(`@[A-Za-z]+\s*\{\s*([^,\s]+)`)
	matches := re.FindAllStringSubmatch(data, -1)
	keys := make([]string, 0, len(matches))
	for _, match := range matches {
		keys = append(keys, match[1])
	}
	sort.Strings(keys)
	return keys
}

func resolveCharterPath(targetDir, root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := targetDir
	if root != "" {
		base = root
	}
	return filepath.Join(base, path)
}

func refFinding(charter Charter, check CharterCheck, file string, line int, ref string) Finding {
	message := check.Message
	if message == "" {
		if ref == "" {
			message = "no references found"
		} else {
			message = fmt.Sprintf("reference %q does not resolve", ref)
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

func stringMapValue(values map[string]any, key string) (string, bool) {
	value, ok := values[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func intMapValue(values map[string]any, key string) (int, bool) {
	value, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func stringSliceMapValue(values map[string]any, key string) []string {
	value, ok := values[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}
