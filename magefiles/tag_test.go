// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCreateReleaseTagCreatesNextDailyTag(t *testing.T) {
	var calls [][]string
	var created []string
	var taggedCommit string
	err := createReleaseTag(
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
		func(args ...string) (string, error) {
			calls = append(calls, append([]string(nil), args...))
			switch strings.Join(args, " ") {
			case "rev-parse --abbrev-ref HEAD":
				return "main", nil
			case "rev-parse HEAD":
				return "abc123", nil
			case "status --porcelain":
				return "", nil
			case "tag -l *v0.20260617.*":
				return strings.Join([]string{
					"v0.20260617.0",
					"applications/catalog/v0.20260617.2",
					"agent-profiles/v0.20260617.1",
					"v0.20260616.9",
					"not-a-release",
				}, "\n"), nil
			default:
				t.Fatalf("unexpected git output args: %q", strings.Join(args, " "))
				return "", nil
			}
		},
		noRemoteTags,
		func(tags []string, commit string) error {
			created = append([]string(nil), tags...)
			taggedCommit = commit
			return nil
		},
		passReleaseGates,
	)
	if err != nil {
		t.Fatalf("createReleaseTag returned error: %v", err)
	}
	want := [][]string{
		{"rev-parse", "--abbrev-ref", "HEAD"},
		{"rev-parse", "HEAD"},
		{"status", "--porcelain"},
		{"rev-parse", "HEAD"},
		{"status", "--porcelain"},
		{"tag", "-l", "*v0.20260617.*"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("git calls = %#v, want %#v", calls, want)
	}
	wantTags := releaseTags("v0.20260617.3", releaseModules())
	if !reflect.DeepEqual(created, wantTags) || taggedCommit != "abc123" {
		t.Fatalf("atomic tags = %v at %s, want %v at abc123",
			created, taggedCommit, wantTags)
	}
}

func TestCreateReleaseTagInGitRepository(t *testing.T) {
	root := initGitRepo(t)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp repo: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	date := "20260617"
	runGit(t, "tag", tagPrefix+date+".0")
	err = createReleaseTag(
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
		gitOutput, noRemoteTags, gitCreateTagSet, passReleaseGates)
	if err != nil {
		t.Fatalf("createReleaseTag returned error: %v", err)
	}
	out := runGitOutput(t, "tag", "-l", tagPrefix+date+".*")
	if !strings.Contains(out, tagPrefix+date+".1") {
		t.Fatalf("local tags = %q, want next daily revision", out)
	}
	for _, mod := range releaseModules() {
		moduleTag := mod + "/" + tagPrefix + date + ".1"
		if !strings.Contains(runGitOutput(t, "tag", "-l", moduleTag), moduleTag) {
			t.Fatalf("local tags missing module tag %q", moduleTag)
		}
	}
	legacyTag := legacyCatalogTagPrefix + tagPrefix + date + ".1"
	if got := strings.TrimSpace(runGitOutput(t, "tag", "-l", legacyTag)); got != legacyTag {
		t.Fatalf("local tags missing compatibility tag %q", legacyTag)
	}
}

func TestCreateReleaseTagRejectsNonMainBranch(t *testing.T) {
	err := createReleaseTag(time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		func(args ...string) (string, error) {
			return "feature/profile-tags", nil
		},
		noRemoteTags,
		func(tags []string, _ string) error {
			t.Fatalf("tag creation called on non-main branch: %v", tags)
			return nil
		},
		passReleaseGates,
	)
	if err == nil {
		t.Fatal("createReleaseTag returned nil error for non-main branch")
	}
	if !strings.Contains(err.Error(), "tag must be run from main") {
		t.Fatalf("error = %q, want branch validation message", err)
	}
}

func TestCreateReleaseTagWrapsTagListingError(t *testing.T) {
	want := errors.New("git tag failed")
	err := createReleaseTag(time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		func(args ...string) (string, error) {
			switch strings.Join(args, " ") {
			case "rev-parse --abbrev-ref HEAD":
				return "main", nil
			case "rev-parse HEAD":
				return "abc123", nil
			case "status --porcelain":
				return "", nil
			}
			return "", want
		},
		noRemoteTags,
		func(tags []string, _ string) error {
			t.Fatalf("tag creation called after listing failure: %v", tags)
			return nil
		},
		passReleaseGates,
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want to wrap %v", err, want)
	}
}

func TestCreateReleaseTagWrapsAtomicTagFailure(t *testing.T) {
	want := errors.New("tag transaction failed")
	err := createReleaseTag(
		time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		successfulReleaseOutput,
		noRemoteTags,
		func(tags []string, commit string) error {
			if !reflect.DeepEqual(tags, releaseTags("v0.20260617.0", releaseModules())) ||
				commit != "abc123" {
				t.Fatalf("atomic request = %v at %s", tags, commit)
			}
			return want
		},
		passReleaseGates,
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want to wrap %v", err, want)
	}
	if !strings.Contains(err.Error(), "atomic release tag set") {
		t.Fatalf("error = %q, want atomic tag context", err)
	}
}

func TestCreateReleaseTagGateFailureCreatesNoTags(t *testing.T) {
	gateErr := errors.New("integration failed")
	var tagCalls int
	err := createReleaseTag(
		time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		successfulReleaseOutput,
		noRemoteTags,
		func(_ []string, _ string) error {
			tagCalls++
			return nil
		},
		func(commit string) error {
			if commit != "abc123" {
				t.Fatalf("gates received commit %q", commit)
			}
			return gateErr
		},
	)
	if !errors.Is(err, gateErr) {
		t.Fatalf("error = %v, want gate failure", err)
	}
	if tagCalls != 0 {
		t.Fatalf("gate failure executed %d tag transactions", tagCalls)
	}
}

func TestGitCreateTagSetIsAtomicOnConflict(t *testing.T) {
	root := initGitRepo(t)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	commit := strings.TrimSpace(runGitOutput(t, "rev-parse", "HEAD"))
	tags := releaseTags("v0.20260617.0", releaseModules())
	runGit(t, "tag", tags[2])
	if err := gitCreateTagSet(tags, commit); err == nil {
		t.Fatal("atomic tag creation succeeded despite conflicting module tag")
	}
	for index, tag := range tags {
		if index == 2 {
			continue
		}
		if got := strings.TrimSpace(runGitOutput(t, "tag", "-l", tag)); got != "" {
			t.Errorf("atomic failure left partial tag %s", got)
		}
	}
	if got := strings.TrimSpace(runGitOutput(t, "tag", "-l", tags[2])); got != tags[2] {
		t.Errorf("pre-existing conflict tag = %q, want %q", got, tags[2])
	}
}

func TestCreateReleaseTagRejectsCommitChangedByGates(t *testing.T) {
	headCalls := 0
	output := func(args ...string) (string, error) {
		if strings.Join(args, " ") == "rev-parse HEAD" {
			headCalls++
			if headCalls == 2 {
				return "def456", nil
			}
		}
		return successfulReleaseOutput(args...)
	}
	var execCalls int
	err := createReleaseTag(
		time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		output,
		noRemoteTags,
		func(_ []string, _ string) error {
			execCalls++
			return nil
		},
		passReleaseGates,
	)
	if err == nil || !strings.Contains(err.Error(), "release commit changed") {
		t.Fatalf("error = %v, want changed-commit rejection", err)
	}
	if execCalls != 0 {
		t.Fatalf("changed commit executed %d tag commands", execCalls)
	}
}

func TestCreateReleaseTagRequiresCleanWorktree(t *testing.T) {
	output := func(args ...string) (string, error) {
		if strings.Join(args, " ") == "status --porcelain" {
			return " M README.md", nil
		}
		return successfulReleaseOutput(args...)
	}
	var gates, execCalls int
	err := createReleaseTag(
		time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		output,
		noRemoteTags,
		func(_ []string, _ string) error {
			execCalls++
			return nil
		},
		func(string) error {
			gates++
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("error = %v, want clean-worktree rejection", err)
	}
	if gates != 0 || execCalls != 0 {
		t.Fatalf("dirty worktree ran gates=%d tag commands=%d", gates, execCalls)
	}
}

func TestCreateReleaseTagRejectsWorktreeChangedByGates(t *testing.T) {
	statusCalls := 0
	output := func(args ...string) (string, error) {
		if strings.Join(args, " ") == "status --porcelain" {
			statusCalls++
			if statusCalls == 2 {
				return " M generated.txt", nil
			}
		}
		return successfulReleaseOutput(args...)
	}
	var execCalls int
	err := createReleaseTag(
		time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		output,
		noRemoteTags,
		func(_ []string, _ string) error {
			execCalls++
			return nil
		},
		passReleaseGates,
	)
	if err == nil || !strings.Contains(err.Error(), "worktree changed while gates ran") {
		t.Fatalf("error = %v, want gate-mutation rejection", err)
	}
	if execCalls != 0 {
		t.Fatalf("changed worktree executed %d tag commands", execCalls)
	}
}

func TestReleaseGatesMatchDocumentedContract(t *testing.T) {
	root := "/release"
	got := releaseGates(root)
	want := []releaseGate{
		{name: "root audit", dir: root, args: []string{"mage", "audit"}},
		{name: "root test", dir: root, args: []string{"mage", "test"}},
		{name: "agent-core integration", dir: "/release/agent-core",
			args: []string{"mage", "integration:all"}},
		{name: "catalog integration", dir: "/release/applications/catalog",
			args: []string{"mage", "integration:all"}},
		{name: "catalog conformance", dir: "/release/applications/catalog",
			args: []string{"mage", "conformance"},
			env:  []string{"AGENT_CORE_ROOT=/release/agent-core"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("release gates = %#v, want %#v", got, want)
	}
}

func TestExecuteReleaseGatesStopsAtFailure(t *testing.T) {
	gateErr := errors.New("tests failed")
	gates := []releaseGate{
		{name: "audit"}, {name: "test"}, {name: "integration"},
	}
	var ran []string
	err := executeReleaseGates(gates, func(gate releaseGate) error {
		ran = append(ran, gate.name)
		if gate.name == "test" {
			return gateErr
		}
		return nil
	})
	if !errors.Is(err, gateErr) {
		t.Fatalf("error = %v, want %v", err, gateErr)
	}
	if !reflect.DeepEqual(ran, []string{"audit", "test"}) {
		t.Fatalf("ran gates = %v, want stop after test", ran)
	}
}

func TestReleaseTags(t *testing.T) {
	got := releaseTags("v0.20260617.0", []string{
		"agent-core", catalogModule, "applications/coding-agent",
		"applications/knowledge-manager-demo",
	})
	want := []string{
		"v0.20260617.0",
		"agent-core/v0.20260617.0",
		"applications/catalog/v0.20260617.0",
		"agent-profiles/v0.20260617.0",
		"applications/coding-agent/v0.20260617.0",
		"applications/knowledge-manager-demo/v0.20260617.0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("releaseTags = %#v, want %#v", got, want)
	}
}

func TestAlignmentReleaseCreatesExactCatalogCompatibilityPair(t *testing.T) {
	got := releaseTags("v0.20260727.0", []string{catalogModule})
	want := []string{
		"v0.20260727.0",
		"applications/catalog/v0.20260727.0",
		"agent-profiles/v0.20260727.0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("alignment release tags = %#v, want exact atomic canonical/legacy pair %#v", got, want)
	}
}

func TestReleaseTagsStopsLegacyCompatibilityAtV1(t *testing.T) {
	got := releaseTags("v1.0.0", []string{catalogModule})
	want := []string{"v1.0.0", "applications/catalog/v1.0.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("releaseTags v1 = %#v, want %#v", got, want)
	}
}

func TestNextRevisionFromTags(t *testing.T) {
	got := nextRevisionFromTags("20260617", strings.Join([]string{
		"v0.20260617.4",
		"applications/catalog/v0.20260617.12",
		"agent-profiles/v0.20260617.9",
		"v0.20260617.bad",
		"v0.20260616.99",
		"v1.20260617.20",
	}, "\n"))
	if got != 13 {
		t.Fatalf("nextRevisionFromTags = %d, want 13", got)
	}
}

func TestNextRevisionFromTagsStartsAtZero(t *testing.T) {
	got := nextRevisionFromTags("20260617", "v0.20260616.1\nnot-a-release")
	if got != 0 {
		t.Fatalf("nextRevisionFromTags empty day = %d, want 0", got)
	}
}

func TestValidateReleaseBranch(t *testing.T) {
	if err := validateReleaseBranch(" main\n"); err != nil {
		t.Fatalf("validateReleaseBranch main returned error: %v", err)
	}
	err := validateReleaseBranch("develop")
	if err == nil {
		t.Fatal("validateReleaseBranch returned nil error for develop")
	}
}

func TestCreateReleaseTagIncludesRemoteTags(t *testing.T) {
	var created []string
	err := createReleaseTag(
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
		successfulReleaseOutput,
		func(string) (string, error) {
			return strings.Join([]string{
				"v0.20260617.0",
				"agent-core/v0.20260617.0",
				"applications/catalog/v0.20260617.2",
			}, "\n"), nil
		},
		func(tags []string, _ string) error {
			created = append([]string(nil), tags...)
			return nil
		},
		passReleaseGates,
	)
	if err != nil {
		t.Fatalf("createReleaseTag returned error: %v", err)
	}
	wantTags := releaseTags("v0.20260617.3", releaseModules())
	if !reflect.DeepEqual(created, wantTags) {
		t.Fatalf("atomic tags = %v, want %v (remote had .0 and .2)", created, wantTags)
	}
}

func TestCreateReleaseTagRemoteFailureCreatesNoTags(t *testing.T) {
	remoteErr := errors.New("network unreachable")
	var tagCalls int
	err := createReleaseTag(
		time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		successfulReleaseOutput,
		func(string) (string, error) { return "", remoteErr },
		func(_ []string, _ string) error {
			tagCalls++
			return nil
		},
		passReleaseGates,
	)
	if !errors.Is(err, remoteErr) {
		t.Fatalf("error = %v, want remote failure", err)
	}
	if !strings.Contains(err.Error(), "remote release tags") {
		t.Fatalf("error = %q, want remote tag context", err)
	}
	if tagCalls != 0 {
		t.Fatalf("remote failure executed %d tag transactions", tagCalls)
	}
}

func TestCreateReleaseTagLocalAndRemoteMaxima(t *testing.T) {
	var created []string
	err := createReleaseTag(
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
		func(args ...string) (string, error) {
			if strings.Join(args, " ") == "tag -l *v0.20260617.*" {
				return "v0.20260617.5", nil
			}
			return successfulReleaseOutput(args...)
		},
		func(string) (string, error) {
			return "v0.20260617.3\nagent-core/v0.20260617.3", nil
		},
		func(tags []string, _ string) error {
			created = append([]string(nil), tags...)
			return nil
		},
		passReleaseGates,
	)
	if err != nil {
		t.Fatalf("createReleaseTag returned error: %v", err)
	}
	wantTags := releaseTags("v0.20260617.6", releaseModules())
	if !reflect.DeepEqual(created, wantTags) {
		t.Fatalf("atomic tags = %v, want %v (local max .5 > remote max .3)", created, wantTags)
	}
}

func TestParseRemoteTagRefs(t *testing.T) {
	input := strings.Join([]string{
		"abc123\trefs/tags/v0.20260617.0",
		"def456\trefs/tags/agent-core/v0.20260617.0",
		"ghi789\trefs/tags/applications/catalog/v0.20260617.0",
		"",
	}, "\n")
	got := parseRemoteTagRefs(input)
	want := strings.Join([]string{
		"v0.20260617.0",
		"agent-core/v0.20260617.0",
		"applications/catalog/v0.20260617.0",
	}, "\n")
	if got != want {
		t.Fatalf("parseRemoteTagRefs = %q, want %q", got, want)
	}
}

func TestMergeTagLines(t *testing.T) {
	got := mergeTagLines(
		"v0.20260617.0\nagent-core/v0.20260617.0",
		"v0.20260617.0\nv0.20260617.1",
	)
	want := "v0.20260617.0\nagent-core/v0.20260617.0\nv0.20260617.1"
	if got != want {
		t.Fatalf("mergeTagLines = %q, want %q", got, want)
	}
}

func passReleaseGates(string) error { return nil }

func noRemoteTags(string) (string, error) { return "", nil }

func successfulReleaseOutput(args ...string) (string, error) {
	switch strings.Join(args, " ") {
	case "rev-parse --abbrev-ref HEAD":
		return "main", nil
	case "rev-parse HEAD":
		return "abc123", nil
	case "status --porcelain", "tag -l *v0.20260617.*":
		return "", nil
	default:
		return "", errors.New("unexpected git output command")
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitInDir(t, root, "init", "-b", "main")
	if err := os.WriteFile(root+"/README.md", []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGitInDir(t, root, "add", "README.md")
	runGitInDir(t, root, "-c", "user.name=Test User", "-c", "user.email=test@example.invalid", "commit", "-m", "init")
	return root
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	if err := exec.Command("git", args...).Run(); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func runGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func runGitOutput(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}
