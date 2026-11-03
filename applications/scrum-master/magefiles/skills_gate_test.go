// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// maxSkillLines is the skill file limit of design.md D10 and eng02.
const maxSkillLines = 50

func TestSkillFilesStayWithinLineLimit(t *testing.T) {
	for _, finding := range oversizedSkills(t, filepath.Join("..", "plugin", "skills")) {
		t.Error(finding)
	}
}

// peopleOnlyWords are the words only a person runs (srd041 R9.1, srd013 R11.5),
// each with the one profile that may declare it.
var peopleOnlyWords = map[string]string{
	"issue assign":     "assign",
	"issue unassign":   "unassign",
	"issue reviewer":   "reviewer",
	"backlog reviewer": "backlog-reviewer",
	"backlog author":   "backlog-author",
	"people review":    "people-review",
	"people unreview":  "people-unreview",
}

// TestOnlyItsOwnProfileRunsAPersonsWord holds that people assign and name who
// signs off, and agents never do: a profile may declare such a word only if it is
// that word's own model-free profile.
func TestOnlyItsOwnProfileRunsAPersonsWord(t *testing.T) {
	root := filepath.Join("..", "plugin", "agents")
	for _, finding := range strayPersonsWords(t, root) {
		t.Error(finding)
	}
	for _, own := range peopleOnlyWords {
		if _, err := os.Stat(filepath.Join(root, own, "declarations.yaml")); err != nil {
			t.Error(err)
		}
		if _, err := os.Stat(filepath.Join(root, own, "llm")); err == nil {
			t.Errorf("%s has a model; only people run it", own)
		}
	}
}

func TestStrayPersonsWordIsReported(t *testing.T) {
	dir := t.TempDir()
	write := func(profile, group, word string) {
		os.MkdirAll(filepath.Join(dir, profile), 0o755)
		body := "tools:\n- name: w\n  args:\n  - " + group + "\n  - " + word + "\n  - --json\n"
		if err := os.WriteFile(filepath.Join(dir, profile, "declarations.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("assign", "issue", "assign")
	write("people-review", "people", "review")
	write("comment", "issue", "comment")
	write("implement", "issue", "assign")
	write("plan", "people", "review")
	got := strayPersonsWords(t, dir)
	if len(got) != 2 || !strings.Contains(got[0], "implement") || !strings.Contains(got[1], "plan") {
		t.Fatalf("findings = %q", got)
	}
}

// strayPersonsWords names the profiles under dir, other than the word's own,
// that declare a word only a person runs.
func strayPersonsWords(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "declarations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var findings []string
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Tools []struct {
				Args []string `yaml:"args"`
			} `yaml:"tools"`
		}
		if err := yaml.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		profile := filepath.Base(filepath.Dir(file))
		for _, tool := range doc.Tools {
			if len(tool.Args) < 2 {
				continue
			}
			word := tool.Args[0] + " " + tool.Args[1]
			if own, ok := peopleOnlyWords[word]; ok && own != profile {
				findings = append(findings, fmt.Sprintf("%s declares %s; only a person runs it, through its own profile", file, word))
			}
		}
	}
	return findings
}

func TestOversizedSkillIsReported(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, lines int) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755)
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < lines; i++ {
			fmt.Fprintln(f, "line")
		}
		f.Close()
	}
	write("small/SKILL.md", maxSkillLines)
	write("large/SKILL.md", maxSkillLines+1)
	write("large/references/long.md", maxSkillLines+10)
	got := oversizedSkills(t, dir)
	if len(got) != 1 || got[0] != fmt.Sprintf("%s: %d lines, limit %d", filepath.Join(dir, "large", "SKILL.md"), maxSkillLines+1, maxSkillLines) {
		t.Fatalf("findings = %q", got)
	}
}

func oversizedSkills(t *testing.T, dir string) []string {
	t.Helper()
	var findings []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if os.IsNotExist(err) && path == dir {
			return filepath.SkipDir
		}
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return err
		}
		n, err := countLines(path)
		if err != nil {
			return err
		}
		if n > maxSkillLines {
			findings = append(findings, fmt.Sprintf("%s: %d lines, limit %d", path, n, maxSkillLines))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return findings
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	return n, s.Err()
}
