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

// TestOnlyItsOwnProfileAssigns holds people assign, agents never (srd041 R9.1):
// a profile may call issue assign or unassign only if it is that word's own
// model-free profile.
func TestOnlyItsOwnProfileAssigns(t *testing.T) {
	root := filepath.Join("..", "plugin", "agents")
	for _, finding := range assigningProfiles(t, root) {
		t.Error(finding)
	}
	for _, own := range []string{"assign", "unassign"} {
		if _, err := os.Stat(filepath.Join(root, own, "declarations.yaml")); err != nil {
			t.Error(err)
		}
		if _, err := os.Stat(filepath.Join(root, own, "llm")); err == nil {
			t.Errorf("%s has a model; only people assign", own)
		}
	}
}

func TestAssigningProfileIsReported(t *testing.T) {
	dir := t.TempDir()
	write := func(profile, word string) {
		os.MkdirAll(filepath.Join(dir, profile), 0o755)
		body := "tools:\n- name: w\n  args:\n  - issue\n  - " + word + "\n  - --json\n"
		if err := os.WriteFile(filepath.Join(dir, profile, "declarations.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("assign", "assign")
	write("unassign", "unassign")
	write("comment", "comment")
	write("implement", "assign")
	got := assigningProfiles(t, dir)
	if len(got) != 1 || !strings.Contains(got[0], "implement") {
		t.Fatalf("findings = %q", got)
	}
}

// assigningProfiles names the profiles under dir, other than the word's own,
// that declare scrum-master issue assign or unassign.
func assigningProfiles(t *testing.T, dir string) []string {
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
			if len(tool.Args) < 2 || tool.Args[0] != "issue" || (tool.Args[1] != "assign" && tool.Args[1] != "unassign") {
				continue
			}
			if tool.Args[1] != profile {
				findings = append(findings, fmt.Sprintf("%s declares issue %s; only people assign, through its own profile", file, tool.Args[1]))
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
