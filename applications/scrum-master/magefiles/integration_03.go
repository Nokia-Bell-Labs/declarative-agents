// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.Install)
}

// Install builds scrum-master, installs the embedded skills for every agent into a fresh git repository, and proves paths, headers, stamps, idempotence, --force, --check, that git ignores exactly the installed files, and that nothing lands outside the agents' directories but the block in .git/info/exclude (test-rel00.3-install). It needs no rig.
func (Integration) Install() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "scrum-master-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	bin, repo := filepath.Join(tmp, "scrum-master"), filepath.Join(tmp, "repo")
	if err := runStreaming(root, "go", "build", "-o", bin, "./magefiles/words"); err != nil {
		return fmt.Errorf("build the words: %w", err)
	}
	if err := exec.Command("git", "init", "-q", repo).Run(); err != nil {
		return err
	}
	if repo, err = filepath.EvalSymlinks(repo); err != nil {
		return err
	}
	os.Remove(filepath.Join(repo, ".git/info/exclude"))
	installJSON := func(into any, args ...string) error {
		cmd := exec.Command(bin, append([]string{"install", "--json"}, args...)...)
		cmd.Dir = repo
		out, err := cmd.Output()
		if into != nil && len(out) > 0 {
			if jerr := json.Unmarshal(out, into); jerr != nil {
				return fmt.Errorf("scrum-master install %s: %v: %s", strings.Join(args, " "), jerr, out)
			}
		}
		return err
	}
	actions := func(res []install.Result) map[string]string {
		m := map[string]string{}
		for _, r := range res {
			rel, _ := filepath.Rel(repo, r.Path)
			m[filepath.ToSlash(rel)] = r.Action
		}
		return m
	}

	c := &check{}
	skills, err := install.EmbeddedSkills()
	if err != nil {
		return err
	}
	var listed []install.Skill
	c.expect(installJSON(&listed, "--list") == nil && len(listed) == len(skills), "--list named %d skills, embedded %d", len(listed), len(skills))

	var first []install.Result
	if err := installJSON(&first, "--agent", "all"); err != nil {
		return err
	}
	want := map[string]string{}
	for _, s := range skills {
		for _, dir := range []string{".cursor/skills/", ".claude/skills/", ".github/skills/"} {
			want[dir+s.Name+"/SKILL.md"] = "created"
		}
	}
	want[".git/info/exclude"] = "created"
	c.expect(fmt.Sprint(actions(first)) == fmt.Sprint(want), "first install: %v, want %v", actions(first), want)
	delete(want, ".git/info/exclude")

	git := func(args ...string) string {
		out, _ := exec.Command("git", append([]string{"-C", repo}, args...)...).Output()
		return strings.TrimSpace(string(out))
	}
	c.expect(git("status", "--porcelain", "--untracked-files=all") == "", "git sees changes after an install: %q", git("status", "--porcelain", "--untracked-files=all"))
	for path := range want {
		c.expect(git("check-ignore", path) == path, "git does not ignore %s", path)
	}
	mine := filepath.Join(repo, ".claude/skills/mine/SKILL.md")
	os.MkdirAll(filepath.Dir(mine), 0o755)
	os.WriteFile(mine, []byte("a skill the repository owns\n"), 0o644)
	c.expect(git("check-ignore", ".claude/skills/mine/SKILL.md") == "", "git ignores a skill scrum-master did not write")
	os.RemoveAll(filepath.Dir(mine))

	var written []string
	filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		if d != nil && d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(repo, p)
			written = append(written, filepath.ToSlash(rel))
		}
		return nil
	})
	c.expect(len(written) == len(want), "files outside the agents' directories: %v", written)

	for path := range want {
		b, _ := os.ReadFile(filepath.Join(repo, path))
		text := string(b)
		name := filepath.Base(filepath.Dir(path))
		head, _, _ := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
		c.expect(strings.HasPrefix(text, "---\nname: "+name+"\ndescription: ") && strings.Contains(text, "\n---\n<!-- skill install version="),
			"%s: not the skill's own Agent Skills front matter followed by the stamp", path)
		c.expect(!strings.Contains(head, "scrum-master install"), "%s: the stamp is inside the front matter", path)
	}

	var second []install.Result
	c.expect(installJSON(&second, "--agent", "all") == nil, "second install failed")
	for path, action := range actions(second) {
		c.expect(action == "unchanged", "second install %s %s", action, path)
	}
	var clean []install.Finding
	c.expect(installJSON(&clean, "--agent", "all", "--check") == nil && len(clean) == 0, "--check after install: %v", clean)

	name := skills[0].Name
	edited := filepath.Join(repo, ".claude/skills", name, "SKILL.md")
	b, _ := os.ReadFile(edited)
	os.WriteFile(edited, append(b, "local edit\n"...), 0o644)
	var findings []install.Finding
	c.expect(installJSON(&findings, "--agent", "all", "--check") != nil, "--check passed with an edited file")
	c.expect(len(findings) == 1 && findings[0].Problem == "edited", "--check findings %v", findings)
	var kept, forced []install.Result
	c.expect(installJSON(&kept, "--agent", "claude") == nil && actions(kept)[".claude/skills/"+name+"/SKILL.md"] == "kept", "edited file not kept: %v", actions(kept))
	c.expect(installJSON(&forced, "--agent", "claude", "--force") == nil && actions(forced)[".claude/skills/"+name+"/SKILL.md"] == "replaced", "--force did not replace: %v", actions(forced))
	after, _ := os.ReadFile(edited)
	c.expect(slices.Equal(after, b), "--force did not restore the rendered skill")
	return c.err("install")
}
