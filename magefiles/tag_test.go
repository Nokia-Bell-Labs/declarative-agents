// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const releaseLockHelperEnv = "DA_TEST_RELEASE_LOCK_HELPER"

func TestReleaseLockRejectsConcurrentProcessAndCleansUp(t *testing.T) {
	if path := os.Getenv(releaseLockHelperEnv); path != "" {
		unlock, err := acquireReleaseLock(path, os.Getpid(), time.Unix(0, 0))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		defer unlock()
		fmt.Println("locked")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}

	path := filepath.Join(t.TempDir(), releaseLockName)
	cmd := exec.Command(os.Args[0], "-test.run=^TestReleaseLockRejectsConcurrentProcessAndCleansUp$")
	cmd.Env = append(os.Environ(), releaseLockHelperEnv+"="+path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatalf("release-lock helper did not become ready: %q (%v)", scanner.Text(), scanner.Err())
	}

	if _, err := acquireReleaseLock(path, os.Getpid(), time.Now()); err == nil {
		t.Fatal("second process acquired an active release lock")
	} else if !strings.Contains(err.Error(), "another release is already active") ||
		!strings.Contains(err.Error(), "pid=") {
		t.Fatalf("concurrent release error = %v, want owner diagnostics", err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	unlock, err := acquireReleaseLock(path, os.Getpid(), time.Now())
	if err != nil {
		t.Fatalf("reacquire after process exit: %v", err)
	}
	unlock()
}

func TestAcquireRepositoryReleaseLockUsesGitPrivatePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), releaseLockName)
	unlock, err := acquireRepositoryReleaseLock(func(args ...string) (string, error) {
		want := []string{"rev-parse", "--git-path", releaseLockName}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("git args = %v, want %v", args, want)
		}
		return path, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(path, "owner")); err != nil {
		t.Fatalf("release lock owner: %v", err)
	}
	unlock()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("release lock remains after unlock: %v", err)
	}
}

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
	wantTags := []string{"v0.20260617.3"}
	if !reflect.DeepEqual(created, wantTags) || taggedCommit != "abc123" {
		t.Fatalf("release tags = %v at %s, want %v at abc123",
			created, taggedCommit, wantTags)
	}
}

func TestVerifyReleaseCommitRunsGatesWithoutTagTransaction(t *testing.T) {
	var calls [][]string
	gateCalls := 0
	commit, err := verifyReleaseCommit(
		func(args ...string) (string, error) {
			calls = append(calls, append([]string(nil), args...))
			switch strings.Join(args, " ") {
			case "rev-parse HEAD":
				return "abc123", nil
			case "status --porcelain":
				return "", nil
			default:
				return "", errors.New("unexpected git command")
			}
		},
		func(commit string) error {
			gateCalls++
			if commit != "abc123" {
				t.Fatalf("gates received commit %q", commit)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if commit != "abc123" || gateCalls != 1 {
		t.Fatalf("dry run commit=%q gateCalls=%d", commit, gateCalls)
	}
	want := [][]string{
		{"rev-parse", "HEAD"},
		{"status", "--porcelain"},
		{"rev-parse", "HEAD"},
		{"status", "--porcelain"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("dry-run git calls = %#v, want %#v", calls, want)
	}
}

func TestVerifyReleaseCommitRejectsDirtyWorktreeBeforeGates(t *testing.T) {
	gates := 0
	_, err := verifyReleaseCommit(
		func(args ...string) (string, error) {
			if strings.Join(args, " ") == "rev-parse HEAD" {
				return "abc123", nil
			}
			return " M generated.txt", nil
		},
		func(string) error {
			gates++
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("dirty dry run error = %v", err)
	}
	if gates != 0 {
		t.Fatalf("dirty dry run executed %d gate sets", gates)
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
	all := strings.Fields(runGitOutput(t, "tag", "-l"))
	if len(all) != 2 {
		t.Fatalf("local tags = %v, want only the seed tag and the new root tag", all)
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
			if !reflect.DeepEqual(tags, []string{"v0.20260617.0"}) ||
				commit != "abc123" {
				t.Fatalf("tag request = %v at %s", tags, commit)
			}
			return want
		},
		passReleaseGates,
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want to wrap %v", err, want)
	}
	if !strings.Contains(err.Error(), "creating release tag") {
		t.Fatalf("error = %q, want tag creation context", err)
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
	tags := []string{"v0.20260617.0", "module-a/v0.20260617.0", "module-b/v0.20260617.0"}
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
		{name: "root audit", dir: root, args: []string{"mage", "audit"}, stage: 0, lane: "root"},
		{name: "root lint", dir: root, args: []string{"mage", "lint"}, stage: 1, lane: "root"},
		{name: "root test", dir: root, args: []string{"mage", "test"},
			env: []string{uiDistReleaseEnv + "=1"}, stage: 2, lane: "root"},
		{name: "agent-core integration", dir: "/release/agent-core",
			args: []string{"mage", "integration:all"}, stage: 3, lane: "agent-core"},
		{name: "catalog integration", dir: "/release/applications/catalog",
			args: []string{"mage", "integration:all"}, stage: 3, lane: "catalog"},
		{name: "catalog conformance", dir: "/release/applications/catalog",
			args: []string{"mage", "conformance"}, stage: 3, lane: "catalog"},
		{name: "applications/chatbot-mesh integration", dir: "/release/applications/chatbot-mesh",
			args: []string{"mage", "integration:all"}, stage: 4, lane: "applications/chatbot-mesh"},
		{name: "applications/coding-agent integration", dir: "/release/applications/coding-agent",
			args: []string{"mage", "integration:all"}, stage: 4, lane: "applications/coding-agent"},
		{name: "applications/agent-architecture integration", dir: "/release/applications/agent-architecture",
			args: []string{"mage", "integration:all"}, stage: 4, lane: "applications/agent-architecture"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("release gates = %#v, want %#v", got, want)
	}
}

func TestChatbotAndArchitectureReleaseGatesCanOverlap(t *testing.T) {
	var selected []releaseGate
	for _, gate := range releaseGates("/release") {
		if gate.name == "applications/chatbot-mesh integration" ||
			gate.name == "applications/agent-architecture integration" {
			selected = append(selected, gate)
		}
	}
	if len(selected) != 2 || selected[0].lane == selected[1].lane {
		t.Fatalf("application release lanes are not independent: %#v", selected)
	}
	started := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- executeReleaseStage(selected, 2, func(gate releaseGate) error {
			started <- gate.name
			<-release
			return nil
		})
	}()
	first, second := <-started, <-started
	got := map[string]bool{first: true, second: true}
	if !got["applications/chatbot-mesh integration"] ||
		!got["applications/agent-architecture integration"] {
		t.Fatalf("concurrent gates = %q, %q", first, second)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestApplicationReleaseStageStartsAllThreeLanes(t *testing.T) {
	var applications []releaseGate
	for _, gate := range releaseGates("/release") {
		if gate.stage == 4 {
			applications = append(applications, gate)
		}
	}
	if len(applications) != 3 {
		t.Fatalf("application gates = %d, want 3", len(applications))
	}
	started := make(chan string, len(applications))
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- executeReleaseStage(
			applications, releaseStageConcurrency,
			func(gate releaseGate) error {
				started <- gate.name
				<-release
				return nil
			})
	}()
	startedLanes := map[string]bool{
		<-started: true,
		<-started: true,
		<-started: true,
	}
	for _, want := range []string{
		"applications/chatbot-mesh integration",
		"applications/coding-agent integration",
		"applications/agent-architecture integration",
	} {
		if !startedLanes[want] {
			t.Fatalf("started lanes = %v, missing %q", startedLanes, want)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestRootTestReleaseGateSignalsReleaseMode guards that the root test gate
// carries the release-mode env so UIDist treats a missing npm as fatal rather
// than a skip (GH-1349); without it a release could pass without rebuilding
// shipped UIs or auditing their dependencies.
func TestRootTestReleaseGateSignalsReleaseMode(t *testing.T) {
	for _, g := range releaseGates("/release") {
		if g.name != "root test" {
			continue
		}
		for _, e := range g.env {
			if e == uiDistReleaseEnv+"=1" {
				return
			}
		}
		t.Fatalf("root test gate env = %v, missing %s=1", g.env, uiDistReleaseEnv)
	}
	t.Fatal("no root test release gate found")
}

// TestReleaseGatesCoverEveryApplicationModule is the orchestration guard: every
// released application module must participate in the release gate through its
// own integration:all aggregate, so a released application can never be tagged
// without its integration evidence running (GH-1343).
func TestReleaseGatesCoverEveryApplicationModule(t *testing.T) {
	gates := releaseGates("/release")
	for _, mod := range applicationModules {
		found := false
		for _, g := range gates {
			if g.name == mod+" integration" &&
				g.dir == filepath.Join("/release", filepath.FromSlash(mod)) &&
				reflect.DeepEqual(g.args, []string{"mage", "integration:all"}) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("release gates missing integration:all participant for application module %q; gates = %#v",
				mod, gates)
		}
	}
}

func TestExecuteReleaseStageRunsIndependentLanesConcurrently(t *testing.T) {
	gates := []releaseGate{
		{name: "a1", lane: "a"},
		{name: "b1", lane: "b"},
		{name: "a2", lane: "a"},
	}
	started := make(chan string, len(gates))
	release := make(chan struct{})
	done := make(chan error, 1)
	var mu sync.Mutex
	var running, maxRunning int
	var calls []string

	go func() {
		done <- executeReleaseStage(gates, 2, func(gate releaseGate) error {
			mu.Lock()
			calls = append(calls, gate.name)
			running++
			if running > maxRunning {
				maxRunning = running
			}
			mu.Unlock()
			started <- gate.name
			if gate.name != "a2" {
				<-release
			}
			mu.Lock()
			running--
			mu.Unlock()
			return nil
		})
	}()

	first, second := <-started, <-started
	if got := map[string]bool{first: true, second: true}; !got["a1"] || !got["b1"] {
		t.Fatalf("first concurrent gates = %q, %q; want a1 and b1", first, second)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxRunning != 2 {
		t.Fatalf("maximum concurrent gates = %d, want 2", maxRunning)
	}
	a1, a2 := slicesIndex(calls, "a1"), slicesIndex(calls, "a2")
	if a1 < 0 || a2 < 0 || a1 >= a2 {
		t.Fatalf("shared lane order = %v, want a1 before a2", calls)
	}
}

func TestReleaseGatesRootFailureBlocksEveryLaterRealGate(t *testing.T) {
	gateErr := errors.New("audit failed")
	started := make(chan string, len(releaseGates("/release")))
	releaseAudit := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- executeReleaseGates(releaseGates("/release"), func(gate releaseGate) error {
			started <- gate.name
			if gate.name == "root audit" {
				<-releaseAudit
				return gateErr
			}
			return nil
		})
	}()

	if first := <-started; first != "root audit" {
		t.Fatalf("first real release gate = %q, want root audit", first)
	}
	select {
	case gate := <-started:
		t.Fatalf("later gate %q started while root audit was blocked", gate)
	default:
	}
	close(releaseAudit)
	if err := <-done; !errors.Is(err, gateErr) {
		t.Fatalf("release error = %v, want %v", err, gateErr)
	}
	select {
	case gate := <-started:
		t.Fatalf("later gate %q started after root audit failed", gate)
	default:
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

func slicesIndex(values []string, want string) int {
	for index, value := range values {
		if value == want {
			return index
		}
	}
	return -1
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
	wantTags := []string{"v0.20260617.3"}
	if !reflect.DeepEqual(created, wantTags) {
		t.Fatalf("release tags = %v, want %v (remote had .0 and .2)", created, wantTags)
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
	wantTags := []string{"v0.20260617.6"}
	if !reflect.DeepEqual(created, wantTags) {
		t.Fatalf("release tags = %v, want %v (local max .5 > remote max .3)", created, wantTags)
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
