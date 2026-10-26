// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var testFS = fstest.MapFS{
	"skills/alpha/SKILL.md": {Data: []byte("---\nname: alpha\ndescription: Do alpha (srd001).\n---\n\n# alpha\n\nRun scrum-master alpha.\n")},
	"skills/beta/SKILL.md":  {Data: []byte("---\nname: beta\ndescription: Do beta.\n---\n# beta\n")},
}

// repoDir is a fresh git repository with no exclude file, whatever the
// local git template holds.
func repoDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	os.Remove(filepath.Join(dir, ".git/info/exclude"))
	return dir
}

func testSkills(t *testing.T) []Skill {
	t.Helper()
	s, err := Skills(testFS, "skills")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func install(t *testing.T, dir string, agents []string, skills []Skill, version string, force bool) map[string]string {
	t.Helper()
	res, err := Install(dir, agents, skills, version, force, false)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, r := range res {
		rel, _ := filepath.Rel(dir, r.Path)
		actions[filepath.ToSlash(rel)] = r.Action
	}
	return actions
}

func TestEmbeddedSkillsParse(t *testing.T) {
	skills, err := EmbeddedSkills()
	if err != nil || len(skills) == 0 {
		t.Fatalf("skills=%v err=%v", skills, err)
	}
	for _, s := range skills {
		if s.Name == "" || s.Description == "" || strings.HasPrefix(s.Body, "---") {
			t.Fatalf("skill %+v", s)
		}
	}
}

// wantSkill is what every agent receives for alpha: the skill's own
// front matter, the stamp after it, then the body.
func wantSkill(t *testing.T, got string) {
	t.Helper()
	head := "---\nname: alpha\ndescription: Do alpha (srd001).\n---\n<!-- skill install version=v1 sha256="
	if !strings.HasPrefix(got, head) || !strings.HasSuffix(got, "-->\n# alpha\n\nRun scrum-master alpha.\n") {
		t.Fatalf("SKILL.md:\n%s", got)
	}
	if ours, edited := stamped(got); !ours || edited {
		t.Fatalf("fresh file reads as ours=%v edited=%v", ours, edited)
	}
}

func TestInstallCursorPathsAndHeaders(t *testing.T) {
	dir := repoDir(t)
	install(t, dir, []string{"cursor"}, testSkills(t), "v1", false)
	wantSkill(t, read(t, filepath.Join(dir, ".cursor/skills/alpha/SKILL.md")))
}

func TestInstallClaudePathsAndHeaders(t *testing.T) {
	dir := repoDir(t)
	install(t, dir, []string{"claude"}, testSkills(t), "v1", false)
	wantSkill(t, read(t, filepath.Join(dir, ".claude/skills/alpha/SKILL.md")))
}

func TestInstallCopilotPathsAndHeaders(t *testing.T) {
	dir := repoDir(t)
	install(t, dir, []string{"copilot"}, testSkills(t), "v1", false)
	wantSkill(t, read(t, filepath.Join(dir, ".github/skills/alpha/SKILL.md")))
}

func TestReinstallChangesNothing(t *testing.T) {
	dir := repoDir(t)
	install(t, dir, Agents, testSkills(t), "v1", false)
	for path, action := range install(t, dir, Agents, testSkills(t), "v1", false) {
		if action != "unchanged" {
			t.Fatalf("%s: %s", path, action)
		}
	}
	if got := install(t, dir, []string{"claude"}, testSkills(t), "v2", false); got[".claude/skills/alpha/SKILL.md"] != "updated" {
		t.Fatalf("a new version rewrites an unedited file: %v", got)
	}
}

func TestReinstallKeepsEditedFile(t *testing.T) {
	dir := repoDir(t)
	install(t, dir, []string{"cursor"}, testSkills(t), "v1", false)
	path := filepath.Join(dir, ".cursor/skills/alpha/SKILL.md")
	edited := read(t, path) + "local note\n"
	os.WriteFile(path, []byte(edited), 0o644)
	if got := install(t, dir, []string{"cursor"}, testSkills(t), "v2", false); got[".cursor/skills/alpha/SKILL.md"] != "kept" {
		t.Fatalf("got %v", got)
	}
	if read(t, path) != edited {
		t.Fatal("edited file was rewritten")
	}
}

func TestReinstallForceReplaces(t *testing.T) {
	dir := repoDir(t)
	path := filepath.Join(dir, ".claude/skills/alpha/SKILL.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("my own alpha\n"), 0o644)
	if got := install(t, dir, []string{"claude"}, testSkills(t), "v1", false); got[".claude/skills/alpha/SKILL.md"] != "kept" {
		t.Fatalf("a file scrum-master did not write is kept: %v", got)
	}
	if got := install(t, dir, []string{"claude"}, testSkills(t), "v1", true); got[".claude/skills/alpha/SKILL.md"] != "replaced" {
		t.Fatalf("got %v", got)
	}
	if read(t, path) != Render("claude", testSkills(t)[0], "v1") {
		t.Fatal("force did not write the rendered skill")
	}
}

func TestReinstallRemovesUneditedStaleSkill(t *testing.T) {
	dir := repoDir(t)
	install(t, dir, Agents, testSkills(t), "v1", false)
	beta := filepath.Join(dir, ".github/skills/beta/SKILL.md")
	alphaCursor := filepath.Join(dir, ".cursor/skills/alpha/SKILL.md")
	os.WriteFile(alphaCursor, []byte(read(t, alphaCursor)+"edit\n"), 0o644)
	mine := filepath.Join(dir, ".cursor/skills/mine/SKILL.md")
	os.MkdirAll(filepath.Dir(mine), 0o755)
	os.WriteFile(mine, []byte("not scrum-master's\n"), 0o644)

	only := []Skill{testSkills(t)[1]}
	got := install(t, dir, Agents, only, "v1", false)
	if got[".github/skills/alpha/SKILL.md"] != "removed" || got[".cursor/skills/alpha/SKILL.md"] != "kept" {
		t.Fatalf("got %v", got)
	}
	if _, err := os.Stat(alphaCursor); err != nil {
		t.Fatal("edited stale skill was removed")
	}
	if _, err := os.Stat(mine); err != nil {
		t.Fatal("a file scrum-master did not write was touched")
	}
	if _, err := os.Stat(beta); err != nil {
		t.Fatal("embedded skill was removed")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	dir := repoDir(t)
	res, err := Install(dir, Agents, testSkills(t), "v1", false, true)
	if err != nil || len(res) != 7 || res[6].Action != "created" {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("dry run wrote %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git/info/exclude")); err == nil {
		t.Fatal("dry run wrote the exclude file")
	}
}

func TestInstallRefusesNonRepository(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(dir, Agents, testSkills(t), "v1", false, false); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("err=%v", err)
	}
	if _, err := Check(dir, Agents, testSkills(t), "v1"); err == nil {
		t.Fatal("check accepted a directory outside git")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("refused install wrote %v", entries)
	}
}

func TestCheckFailsOnMissingOutdatedEdited(t *testing.T) {
	dir := repoDir(t)
	skills := testSkills(t)
	install(t, dir, []string{"claude"}, skills, "v1", false)
	os.Remove(filepath.Join(dir, ".claude/skills/beta/SKILL.md"))
	install(t, dir, []string{"cursor"}, skills, "v1", false)
	cursorAlpha := filepath.Join(dir, ".cursor/skills/alpha/SKILL.md")
	os.WriteFile(cursorAlpha, []byte(read(t, cursorAlpha)+"edit\n"), 0o644)
	install(t, dir, []string{"copilot"}, append(skills, Skill{Name: "gone", Description: "Gone.", Body: "x\n"}), "v2", false)

	findings, err := Check(dir, Agents, skills, "v2")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range findings {
		rel, _ := filepath.Rel(dir, f.Path)
		got[filepath.ToSlash(rel)] = f.Problem
	}
	want := map[string]string{
		".claude/skills/alpha/SKILL.md": "outdated",
		".claude/skills/beta/SKILL.md":  "missing",
		".cursor/skills/alpha/SKILL.md": "edited",
		".cursor/skills/beta/SKILL.md":  "outdated",
		".github/skills/gone/SKILL.md":  "stale",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCheckPassesAfterInstall(t *testing.T) {
	dir := repoDir(t)
	install(t, dir, Agents, testSkills(t), "v1", false)
	if findings, err := Check(dir, Agents, testSkills(t), "v1"); err != nil || len(findings) != 0 {
		t.Fatalf("findings=%v err=%v", findings, err)
	}
}

func TestInstallIgnoresOwnedFiles(t *testing.T) {
	dir := repoDir(t)
	ignore := filepath.Join(dir, ".git/info/exclude")
	os.WriteFile(ignore, []byte("/build/\n"), 0o644)
	mine := filepath.Join(dir, ".claude/skills/mine/SKILL.md")
	os.MkdirAll(filepath.Dir(mine), 0o755)
	os.WriteFile(mine, []byte("not scrum-master's\n"), 0o644)

	got := install(t, dir, []string{"cursor", "claude"}, testSkills(t), "v1", false)
	if got[".git/info/exclude"] != "updated" {
		t.Fatalf("got %v", got)
	}
	want := "/build/\n\n" + ignoreBegin + "\n" +
		"/.claude/skills/alpha/SKILL.md\n/.claude/skills/beta/SKILL.md\n" +
		"/.cursor/skills/alpha/SKILL.md\n/.cursor/skills/beta/SKILL.md\n" + ignoreEnd + "\n"
	if read(t, ignore) != want {
		t.Fatalf("exclude:\n%s", read(t, ignore))
	}
	if _, ok := install(t, dir, []string{"cursor"}, testSkills(t), "v1", false)[".git/info/exclude"]; ok {
		t.Fatal("an unchanged block was rewritten")
	}

	os.WriteFile(ignore, []byte(read(t, ignore)+"/tmp/\n"), 0o644)
	install(t, dir, Agents, []Skill{testSkills(t)[1]}, "v1", false)
	want = "/build/\n\n" + ignoreBegin + "\n" +
		"/.claude/skills/beta/SKILL.md\n/.cursor/skills/beta/SKILL.md\n/.github/skills/beta/SKILL.md\n" +
		ignoreEnd + "\n/tmp/\n"
	if read(t, ignore) != want {
		t.Fatalf("after the stale skill went:\n%s", read(t, ignore))
	}
}

func TestCheckFailsOnUnignored(t *testing.T) {
	dir := repoDir(t)
	install(t, dir, []string{"claude"}, testSkills(t), "v1", false)
	os.WriteFile(filepath.Join(dir, ".git/info/exclude"), []byte("/build/\n"), 0o644)
	findings, err := Check(dir, []string{"claude"}, testSkills(t), "v1")
	if err != nil || len(findings) != 2 || findings[0].Problem != "unignored" || findings[1].Problem != "unignored" {
		t.Fatalf("findings=%v err=%v", findings, err)
	}
	install(t, dir, []string{"claude"}, testSkills(t), "v1", false)
	if findings, _ := Check(dir, []string{"claude"}, testSkills(t), "v1"); len(findings) != 0 {
		t.Fatalf("after install: %v", findings)
	}
}

func TestInstallInWorktreeUsesTheCloneExclude(t *testing.T) {
	main := repoDir(t)
	wt := filepath.Join(filepath.Dir(main), "wt")
	git := func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(main, "commit", "-q", "--allow-empty", "-m", "init")
	git(main, "worktree", "add", "-q", wt)

	got := install(t, wt, []string{"claude"}, testSkills(t), "v1", false)
	if got["../"+filepath.Base(main)+"/.git/info/exclude"] != "created" {
		t.Fatalf("got %v", got)
	}
	if status := git(wt, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Fatalf("worktree status %q", status)
	}
}

// legacyFile is a command file an earlier release wrote: its header, a stamp
// over the rest, and its body.
func legacyFile(header, body string) string {
	return header + "<!-- tpm install version=v0 sha256=" + digest(header+body) + " -->\n" + body
}

func TestInstallMigratesCommandFiles(t *testing.T) {
	dir := repoDir(t)
	put := func(rel, content string) string {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
		return p
	}
	claudeOld := put(".claude/commands/alpha.md", legacyFile("---\ndescription: Do alpha.\n---\n", "# alpha\n"))
	copilotOld := put(".github/prompts/alpha.prompt.md", legacyFile("---\nagent: agent\ndescription: Do alpha.\n---\n", "# alpha\n"))
	cursorEdited := put(".cursor/commands/beta.md", legacyFile("", "# beta\n")+"my note\n")
	mine := put(".claude/commands/mine.md", "a command the repository owns\n")
	if got := InstalledAgents(dir); len(got) != 3 {
		t.Fatalf("agents found from the earlier layout: %v", got)
	}

	got := install(t, dir, Agents, testSkills(t), "v1", false)
	if got[".claude/commands/alpha.md"] != "removed" || got[".github/prompts/alpha.prompt.md"] != "removed" || got[".cursor/commands/beta.md"] != "kept" {
		t.Fatalf("migration: %v", got)
	}
	for _, gone := range []string{claudeOld, copilotOld} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s was not removed", gone)
		}
	}
	for _, kept := range []string{cursorEdited, mine} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was removed", kept)
		}
	}
	wantSkill(t, read(t, filepath.Join(dir, ".claude/skills/alpha/SKILL.md")))
	exclude := read(t, filepath.Join(dir, ".git/info/exclude"))
	if !strings.Contains(exclude, "/.cursor/commands/beta.md") || strings.Contains(exclude, "/.claude/commands/alpha.md") || strings.Contains(exclude, "mine.md") {
		t.Fatalf("exclude block:\n%s", exclude)
	}
	findings, err := Check(dir, Agents, testSkills(t), "v1")
	if err != nil || len(findings) != 1 || findings[0].Problem != "stale" || !strings.HasSuffix(findings[0].Path, "beta.md") {
		t.Fatalf("check after migration: %v %v", findings, err)
	}
}

// detect names the agents a repository already uses, in Agents' order,
// and refuses when it uses none; .github alone, which CI uses, is not
// copilot (GH-49).
func TestResolveAgentsDetectsTheAgentsInUse(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755)
	if _, err := ResolveAgents([]string{Detect}, dir); err == nil || !strings.Contains(err.Error(), "--agent") {
		t.Fatalf("detect in a repository using no agent = %v, want a refusal naming --agent", err)
	}
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("x\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".github", "skills"), 0o755)
	got, err := ResolveAgents([]string{Detect}, dir)
	if err != nil || strings.Join(got, " ") != "claude copilot" {
		t.Fatalf("detect = %v, %v, want claude copilot", got, err)
	}
	if got, err := ResolveAgents([]string{"cursor"}, dir); err != nil || strings.Join(got, " ") != "cursor" {
		t.Fatalf("an explicit agent = %v, %v", got, err)
	}
}

// With no release's profiles installed, the source check refuses and says
// nothing was written (GH-49).
func TestCheckSourceRefusesWithNoProfilesInstalled(t *testing.T) {
	t.Setenv(SourceEnv, "")
	t.Setenv(HomeEnv, t.TempDir())
	if err := CheckSource(t.TempDir()); err == nil || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("CheckSource = %v, want a refusal", err)
	}
}
