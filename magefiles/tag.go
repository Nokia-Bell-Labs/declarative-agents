// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	tagPrefix       = "v0."
	baseBranch      = "main"
	catalogModule   = "applications/catalog"
	releaseLockName = "declarative-agents-release.lock"
)

type releaseGate struct {
	name string
	dir  string
	args []string
	env  []string
	// Gates in one stage may overlap. Gates sharing a lane remain serial
	// because they use the same fixed ports or local infrastructure.
	stage int
	lane  string
}

type releaseGateRunner func(string) error
type releaseCommandRunner func(releaseGate) error

type remoteTagsFunc func(date string) (string, error)

const releaseStageConcurrency = 2

// Tag creates the single repository-wide release tag.
func Tag() error {
	unlock, err := acquireRepositoryReleaseLock(gitOutput)
	if err != nil {
		return err
	}
	defer unlock()
	return createReleaseTag(time.Now(), gitOutput, gitRemoteTags, gitCreateTagSet, runReleaseGates)
}

func acquireRepositoryReleaseLock(output gitOutputFunc) (func(), error) {
	path, err := output("rev-parse", "--git-path", releaseLockName)
	if err != nil {
		return nil, fmt.Errorf("resolve release lock path: %w", err)
	}
	path, err = filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("resolve absolute release lock path: %w", err)
	}
	return acquireReleaseLock(path, os.Getpid(), time.Now())
}

func acquireReleaseLock(path string, pid int, started time.Time) (func(), error) {
	if err := os.Mkdir(path, 0o700); err != nil {
		if os.IsExist(err) {
			owner, readErr := os.ReadFile(filepath.Join(path, "owner"))
			if readErr != nil {
				owner = []byte("owner metadata unavailable")
			}
			return nil, fmt.Errorf(
				"another release is already active (%s; lock %s); "+
					"remove the lock only after confirming no mage tag process is running",
				strings.TrimSpace(string(owner)), path)
		}
		return nil, fmt.Errorf("acquire release lock %s: %w", path, err)
	}

	owner := fmt.Sprintf("pid=%d started=%s",
		pid, started.UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(path, "owner"), []byte(owner+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("write release lock owner: %w", err)
	}
	return func() {
		if err := os.RemoveAll(path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: remove release lock %s: %v\n", path, err)
		}
	}, nil
}

func createReleaseTag(
	now time.Time,
	output gitOutputFunc,
	remoteTags remoteTagsFunc,
	createTags gitTagSetFunc,
	runGates releaseGateRunner,
) error {
	branch, err := output("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if err := validateReleaseBranch(branch); err != nil {
		return err
	}
	commit, err := output("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolving release commit: %w", err)
	}
	status, err := output("status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking release worktree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return errors.New("tag requires a clean worktree")
	}
	if err := runGates(commit); err != nil {
		return fmt.Errorf("release gates for commit %s: %w", commit, err)
	}
	afterGates, err := output("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("verifying release commit after gates: %w", err)
	}
	if strings.TrimSpace(afterGates) != strings.TrimSpace(commit) {
		return fmt.Errorf("release commit changed while gates ran: started %s, now %s",
			commit, afterGates)
	}
	status, err = output("status", "--porcelain")
	if err != nil {
		return fmt.Errorf("verifying release worktree after gates: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return errors.New("release worktree changed while gates ran")
	}

	date := now.Format("20060102")
	localTags, err := output("tag", "-l", "*"+tagPrefix+date+".*")
	if err != nil {
		return fmt.Errorf("listing local release tags: %w", err)
	}
	remoteTagOutput, err := remoteTags(date)
	if err != nil {
		return fmt.Errorf("listing remote release tags: %w", err)
	}
	mergedTags := mergeTagLines(localTags, remoteTagOutput)
	tag := fmt.Sprintf("%s%s.%d", tagPrefix, date, nextRevisionFromTags(date, mergedTags))

	fmt.Printf("creating release tag %s\n", tag)
	if err := createTags([]string{tag}, commit); err != nil {
		return fmt.Errorf("creating release tag: %w", err)
	}
	fmt.Printf("done — created %s\n", tag)
	return nil
}

func runReleaseGates(commit string) error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	fmt.Printf("release: verifying commit %s\n", commit)
	return executeReleaseGates(releaseGates(root), runReleaseCommand)
}

func releaseGates(root string) []releaseGate {
	catalogRoot := filepath.Join(root, catalogModule)
	gates := []releaseGate{
		{name: "root audit", dir: root, args: []string{"mage", "audit"}, stage: 0, lane: "root"},
		// Lint gates a release from GH-1479 on. It could not before: the policy
		// had never run, and its first run reported findings, which GH-1481
		// cleared. It sits early because it is the cheapest gate here, and a
		// policy violation should not wait behind the integration aggregates.
		{name: "root lint", dir: root, args: []string{"mage", "lint"}, stage: 1, lane: "root"},
		// DA_RELEASE_GATE makes the UI reproducibility gate treat a missing npm
		// as fatal rather than a developer skip (GH-1349), so a release cannot
		// pass without rebuilding shipped UIs and auditing their dependencies.
		{name: "root test", dir: root, args: []string{"mage", "test"},
			env: []string{uiDistReleaseEnv + "=1"}, stage: 2, lane: "root"},
		{name: "agent-core integration", dir: filepath.Join(root, "agent-core"),
			args: []string{"mage", "integration:all"}, stage: 3, lane: "agent-core"},
		{name: "catalog integration", dir: catalogRoot,
			args: []string{"mage", "integration:all"}, stage: 3, lane: "catalog"},
		{name: "catalog conformance", dir: catalogRoot,
			args: []string{"mage", "conformance"}, stage: 3, lane: "catalog"},
	}
	// Every released application module must enter the release gate through its
	// own integration:all aggregate; otherwise an application is tagged without
	// its application-owned integration evidence ever running (GH-1343).
	for _, mod := range applicationModules {
		lane := mod
		// Chatbot Mesh and Agent Architecture use the same collector ports
		// (18191-18193). Keep them in one lane while Coding Agent (182xx) and
		// Prose Editor's dynamically allocated listeners run independently.
		if mod == "applications/agent-architecture" {
			lane = "applications/chatbot-mesh"
		}
		gates = append(gates, releaseGate{
			name:  mod + " integration",
			dir:   filepath.Join(root, filepath.FromSlash(mod)),
			args:  []string{"mage", "integration:all"},
			stage: 4,
			lane:  lane,
		})
	}
	return gates
}

func executeReleaseGates(gates []releaseGate, run releaseCommandRunner) error {
	for first := 0; first < len(gates); {
		last := first + 1
		for last < len(gates) && gates[last].stage == gates[first].stage {
			last++
		}
		if err := executeReleaseStage(gates[first:last], releaseStageConcurrency, run); err != nil {
			return err
		}
		first = last
	}
	return nil
}

type releaseLane struct {
	index int
	gates []releaseGate
}

type releaseLaneResult struct {
	index int
	err   error
}

func executeReleaseStage(
	gates []releaseGate,
	limit int,
	run releaseCommandRunner,
) error {
	if limit < 1 {
		limit = 1
	}
	lanes := releaseLanes(gates)
	if len(lanes) == 0 {
		return nil
	}

	results := make(chan releaseLaneResult, len(lanes))
	next, active := 0, 0
	launch := func(lane releaseLane) {
		active++
		go func() {
			results <- releaseLaneResult{index: lane.index, err: executeReleaseLane(lane.gates, run)}
		}()
	}
	for next < len(lanes) && active < limit {
		launch(lanes[next])
		next++
	}

	var failures []releaseLaneResult
	for active > 0 {
		result := <-results
		active--
		if result.err != nil {
			failures = append(failures, result)
		}
		if len(failures) == 0 && next < len(lanes) {
			launch(lanes[next])
			next++
		}
	}
	if len(failures) == 0 {
		return nil
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].index < failures[j].index })
	return failures[0].err
}

func releaseLanes(gates []releaseGate) []releaseLane {
	indexByName := make(map[string]int)
	var lanes []releaseLane
	for _, gate := range gates {
		laneName := gate.lane
		if laneName == "" {
			laneName = "serial"
		}
		index, ok := indexByName[laneName]
		if !ok {
			index = len(lanes)
			indexByName[laneName] = index
			lanes = append(lanes, releaseLane{index: index})
		}
		lanes[index].gates = append(lanes[index].gates, gate)
	}
	return lanes
}

func executeReleaseLane(gates []releaseGate, run releaseCommandRunner) error {
	for _, gate := range gates {
		started := time.Now()
		fmt.Printf("=== release gate: %s ===\n", gate.name)
		if err := run(gate); err != nil {
			elapsed := time.Since(started).Round(time.Millisecond)
			return fmt.Errorf("%s failed after %s: %w", gate.name, elapsed, err)
		}
		fmt.Printf("=== release gate complete: %s (%s) ===\n",
			gate.name, time.Since(started).Round(time.Millisecond))
	}
	return nil
}

func runReleaseCommand(gate releaseGate) error {
	cmd := exec.Command(gate.args[0], gate.args[1:]...)
	cmd.Dir = gate.dir
	cmd.Env = append(os.Environ(), gate.env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func validateReleaseBranch(branch string) error {
	current := strings.TrimSpace(branch)
	if current != baseBranch {
		return fmt.Errorf("tag must be run from %s (currently on %s)", baseBranch, current)
	}
	return nil
}

func nextRevisionFromTags(date, tags string) int {
	// Releases before GH-1373 created module-scoped tags (agent-core/v0.*,
	// applications/catalog/v0.*, agent-profiles/v0.*) at the same dates, so
	// the pattern tolerates any prefix when computing the next revision.
	revRe := regexp.MustCompile(`^(?:[^/]+/)*` + regexp.QuoteMeta(tagPrefix) +
		regexp.QuoteMeta(date) + `\.(\d+)$`)
	maxRev := -1
	for _, line := range strings.Split(tags, "\n") {
		m := revRe.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) != 2 {
			continue
		}
		rev, err := strconv.Atoi(m[1])
		if err == nil && rev > maxRev {
			maxRev = rev
		}
	}
	return maxRev + 1
}

func gitRemoteTags(date string) (string, error) {
	out, err := exec.Command("git", "ls-remote", "--tags", "origin",
		"*"+tagPrefix+date+"*").Output()
	if err != nil {
		return "", err
	}
	return parseRemoteTagRefs(string(out)), nil
}

func parseRemoteTagRefs(lsRemoteOutput string) string {
	var tags []string
	for _, line := range strings.Split(lsRemoteOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		ref := strings.TrimPrefix(parts[1], "refs/tags/")
		tags = append(tags, ref)
	}
	return strings.Join(tags, "\n")
}

func mergeTagLines(sets ...string) string {
	seen := make(map[string]bool)
	var merged []string
	for _, set := range sets {
		for _, line := range strings.Split(set, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || seen[line] {
				continue
			}
			seen[line] = true
			merged = append(merged, line)
		}
	}
	return strings.Join(merged, "\n")
}

type gitOutputFunc func(args ...string) (string, error)
type gitTagSetFunc func([]string, string) error

func gitCreateTagSet(tags []string, commit string) error {
	var transaction strings.Builder
	transaction.WriteString("start\n")
	for _, tag := range tags {
		fmt.Fprintf(&transaction, "create refs/tags/%s %s\n", tag, commit)
	}
	transaction.WriteString("prepare\ncommit\n")
	cmd := exec.Command("git", "update-ref", "--stdin")
	cmd.Stdin = strings.NewReader(transaction.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
