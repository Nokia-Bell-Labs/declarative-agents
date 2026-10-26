// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"
)

// The Agent Plugins 1.0 manifest (agent-plugins.org/specification §5) and
// the Agent Skills front matter (agentskills.io/specification) that every
// SKILL.md under plugin/skills must follow (srd037 R1.1, R1.3).
const pluginSchema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"

var (
	manifestFields  = []string{"$schema", "name", "version", "description", "author", "homepage", "repository", "license", "keywords", "extensions"}
	authorFields    = []string{"name", "email", "url"}
	skillFields     = []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}
	pluginNameRule  = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
	skillNameRule   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	reverseDomainNS = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)+$`)
)

func manifestProblems(b []byte) []string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return []string{"plugin.json is not a JSON object: " + err.Error()}
	}
	var out []string
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !contains(manifestFields, k) {
			out = append(out, fmt.Sprintf("plugin.json: field %q is not in the Agent Plugins 1.0 manifest", k))
		}
	}
	str := func(k string, required bool) (string, bool) {
		raw, ok := m[k]
		if !ok {
			if required {
				out = append(out, fmt.Sprintf("plugin.json: required field %q is missing", k))
			}
			return "", false
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			out = append(out, fmt.Sprintf("plugin.json: %q is not a string", k))
			return "", false
		}
		return s, true
	}
	if s, ok := str("$schema", true); ok && s != pluginSchema {
		out = append(out, fmt.Sprintf("plugin.json: $schema %q is not %s", s, pluginSchema))
	}
	if name, ok := str("name", true); ok {
		if len(name) < 1 || len(name) > 64 || !pluginNameRule.MatchString(name) || strings.Contains(name, "--") || strings.Contains(name, "..") {
			out = append(out, fmt.Sprintf("plugin.json: name %q breaks the plugin name rules", name))
		}
	}
	for _, k := range []string{"version", "description", "homepage", "repository", "license"} {
		str(k, false)
	}
	if raw, ok := m["keywords"]; ok {
		var kw []string
		if err := json.Unmarshal(raw, &kw); err != nil {
			out = append(out, "plugin.json: keywords is not a list of strings")
		}
	}
	if raw, ok := m["author"]; ok {
		var a map[string]json.RawMessage
		if err := json.Unmarshal(raw, &a); err != nil {
			out = append(out, "plugin.json: author is not an object")
		}
		for k, v := range a {
			var s string
			if !contains(authorFields, k) || json.Unmarshal(v, &s) != nil {
				out = append(out, fmt.Sprintf("plugin.json: author.%s is not one of name, email, url as a string", k))
			}
		}
	}
	if raw, ok := m["extensions"]; ok {
		var ext map[string]map[string]any
		if err := json.Unmarshal(raw, &ext); err != nil {
			out = append(out, "plugin.json: extensions is not an object of objects")
		}
		for ns := range ext {
			if !reverseDomainNS.MatchString(ns) {
				out = append(out, fmt.Sprintf("plugin.json: extensions key %q is not a reverse-domain namespace", ns))
			}
		}
	}
	return out
}

// skillProblems checks every skills/<name>/SKILL.md of a plugin root. An
// absent skills directory is allowed: every skill is a declarative-agents
// profile under agents/, and its SKILL.md is rendered at install
// (design.md D14, GH-198).
func skillProblems(fsys fs.FS) []string {
	var out []string
	entries, err := fs.ReadDir(fsys, "skills")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []string{"skills/: " + err.Error()}
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := path.Join("skills", e.Name(), "SKILL.md")
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			out = append(out, fmt.Sprintf("%s: no SKILL.md", path.Dir(p)))
			continue
		}
		out = append(out, frontMatterProblems(p, e.Name(), b)...)
	}
	fs.WalkDir(fsys, "skills", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "SKILL.md" && strings.Count(p, "/") != 2 {
			out = append(out, fmt.Sprintf("%s: a skill must sit exactly one level under skills/", p))
		}
		return nil
	})
	return out
}

func frontMatterProblems(p, dir string, b []byte) []string {
	text := string(b)
	rest, ok := strings.CutPrefix(text, "---\n")
	head, _, closed := strings.Cut(rest, "\n---\n")
	if !ok || !closed {
		return []string{p + ": SKILL.md must open with YAML front matter"}
	}
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(head), &fm); err != nil {
		return []string{p + ": front matter: " + err.Error()}
	}
	var out []string
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !contains(skillFields, k) {
			out = append(out, fmt.Sprintf("%s: front matter key %q is not in the Agent Skills specification", p, k))
		}
	}
	name, _ := fm["name"].(string)
	switch {
	case name == "":
		out = append(out, p+": name is required")
	case len(name) > 64 || !skillNameRule.MatchString(name):
		out = append(out, fmt.Sprintf("%s: name %q breaks the Agent Skills name rules", p, name))
	case name != dir:
		out = append(out, fmt.Sprintf("%s: name %q is not its directory %q", p, name, dir))
	}
	desc, _ := fm["description"].(string)
	if n := len([]rune(strings.TrimSpace(desc))); n < 1 || n > 1024 {
		out = append(out, fmt.Sprintf("%s: description has %d characters; Agent Skills needs 1 to 1024", p, n))
	}
	if c, ok := fm["compatibility"]; ok {
		if s, isStr := c.(string); !isStr || len(s) < 1 || len(s) > 500 {
			out = append(out, p+": compatibility must be a string of 1 to 500 characters")
		}
	}
	if md, ok := fm["metadata"]; ok {
		m, isMap := md.(map[string]any)
		for _, v := range m {
			if _, isStr := v.(string); !isStr {
				isMap = false
			}
		}
		if !isMap {
			out = append(out, p+": metadata must map strings to strings")
		}
	}
	for _, k := range []string{"license", "allowed-tools"} {
		if v, ok := fm[k]; ok {
			if _, isStr := v.(string); !isStr {
				out = append(out, fmt.Sprintf("%s: %s must be a string", p, k))
			}
		}
	}
	return out
}

// escapingLinks are symbolic links under root that resolve outside it
// (Agent Plugins §4.1).
func escapingLinks(root string) []string {
	abs, _ := filepath.Abs(root)
	var out []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink == 0 {
			return err
		}
		target, err := filepath.EvalSymlinks(p)
		if err != nil || !strings.HasPrefix(target, abs+string(filepath.Separator)) {
			out = append(out, p+": resolves outside the plugin root")
		}
		return nil
	})
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestPluginManifestConforms(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range manifestProblems(b) {
		t.Error(p)
	}
	for _, p := range escapingLinks(filepath.Join("..", "plugin")) {
		t.Error(p)
	}
}

func TestSkillsConformToAgentSkills(t *testing.T) {
	for _, p := range skillProblems(os.DirFS(filepath.Join("..", "plugin"))) {
		t.Error(p)
	}
}

func TestManifestRulesRejectBrokenManifests(t *testing.T) {
	good := `{"$schema": "` + pluginSchema + `", "name": "acme.tools"}`
	if p := manifestProblems([]byte(good)); len(p) != 0 {
		t.Fatalf("minimal manifest: %v", p)
	}
	for name, bad := range map[string]string{
		"no schema":      `{"name": "a"}`,
		"other schema":   `{"$schema": "https://agent-plugins.org/schemas/2.0.0/plugin.schema.json", "name": "a"}`,
		"no name":        `{"$schema": "` + pluginSchema + `"}`,
		"capital name":   `{"$schema": "` + pluginSchema + `", "name": "My-Plugin"}`,
		"double hyphen":  `{"$schema": "` + pluginSchema + `", "name": "has--double"}`,
		"leading hyphen": `{"$schema": "` + pluginSchema + `", "name": "-start"}`,
		"unknown field":  `{"$schema": "` + pluginSchema + `", "name": "a", "skills": []}`,
		"author field":   `{"$schema": "` + pluginSchema + `", "name": "a", "author": {"name": "x", "twitter": "y"}}`,
		"keywords type":  `{"$schema": "` + pluginSchema + `", "name": "a", "keywords": "x"}`,
		"extension ns":   `{"$schema": "` + pluginSchema + `", "name": "a", "extensions": {"client": {}}}`,
		"not an object":  `["a"]`,
	} {
		if p := manifestProblems([]byte(bad)); len(p) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSkillRulesRejectBrokenSkills(t *testing.T) {
	skill := func(fm string) *fstest.MapFile { return &fstest.MapFile{Data: []byte("---\n" + fm + "\n---\nbody\n")} }
	good := fstest.MapFS{"skills/pdf-tools/SKILL.md": skill("name: pdf-tools\ndescription: Read PDFs. Use when a person shares a PDF.\nmetadata:\n  version: \"1\"")}
	if p := skillProblems(good); len(p) != 0 {
		t.Fatalf("good skill: %v", p)
	}
	for name, bad := range map[string]fstest.MapFS{
		"name not dir":        {"skills/a/SKILL.md": skill("name: b\ndescription: d")},
		"capital name":        {"skills/Pdf/SKILL.md": skill("name: Pdf\ndescription: d")},
		"double hyphen":       {"skills/a--b/SKILL.md": skill("name: a--b\ndescription: d")},
		"no description":      {"skills/a/SKILL.md": skill("name: a")},
		"long description":    {"skills/a/SKILL.md": skill("name: a\ndescription: " + strings.Repeat("x", 1025))},
		"unknown key":         {"skills/a/SKILL.md": skill("name: a\ndescription: d\nargument-hint: x")},
		"no front matter":     {"skills/a/SKILL.md": {Data: []byte("# a\n")}},
		"nested skill":        {"skills/a/SKILL.md": skill("name: a\ndescription: d"), "skills/a/b/SKILL.md": skill("name: b\ndescription: d")},
		"missing SKILL.md":    {"skills/a/README.md": {Data: []byte("x")}},
		"metadata not flat":   {"skills/a/SKILL.md": skill("name: a\ndescription: d\nmetadata:\n  n: 1")},
		"empty compatibility": {"skills/a/SKILL.md": skill("name: a\ndescription: d\ncompatibility: \"\"")},
	} {
		if p := skillProblems(bad); len(p) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}
