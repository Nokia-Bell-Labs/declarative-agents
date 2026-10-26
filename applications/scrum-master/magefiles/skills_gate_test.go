// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// maxSkillLines is the skill file limit of design.md D10 and eng02.
const maxSkillLines = 50

func TestSkillFilesStayWithinLineLimit(t *testing.T) {
	for _, finding := range oversizedSkills(t, filepath.Join("..", "plugin", "skills")) {
		t.Error(finding)
	}
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
