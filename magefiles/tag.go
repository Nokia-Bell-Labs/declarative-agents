// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
	"sync/atomic"
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
	// Gates sharing a lane remain serial because they use the same fixed ports
	// or local infrastructure. Independent lanes may overlap when every
	// requested resource has capacity.
	lane      string
	resources []releaseResourceClass
	priority  int
	exclusive bool
}

type releaseGateRunner func(string) error
type releaseCommandRunner func(releaseGate) error

type remoteTagsFunc func(date string) (string, error)

type releaseResourceClass string

const (
	releaseResourceDocker     releaseResourceClass = "docker"
	releaseResourceHostOllama releaseResourceClass = "host-ollama"
	releaseResourceCPU        releaseResourceClass = "cpu"

	chatbotReleasePriority = 100
)

var releaseResourceCapacities = map[releaseResourceClass]int{
	releaseResourceDocker:     3,
	releaseResourceHostOllama: 1,
	releaseResourceCPU:        1,
}

// Tag creates the single repository-wide release tag.
func Tag() error {
	unlock, err := acquireRepositoryReleaseLock(gitOutput)
	if err != nil {
		return err
	}
	defer unlock()
	return createReleaseTag(time.Now(), gitOutput, gitRemoteTags, gitCreateTagSet, runReleaseGates)
}

// TagDryRun executes the exact release gates against one clean, pinned commit
// without creating a tag.
func TagDryRun() error {
	unlock, err := acquireRepositoryReleaseLock(gitOutput)
	if err != nil {
		return err
	}
	defer unlock()
	started := time.Now()
	commit, err := verifyReleaseCommit(gitOutput, runReleaseGates)
	if err != nil {
		return err
	}
	fmt.Printf("release dry run complete: commit=%s elapsed=%s\n",
		commit, time.Since(started).Round(time.Millisecond))
	return nil
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
	commit, err := verifyReleaseCommit(output, runGates)
	if err != nil {
		return err
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

func verifyReleaseCommit(
	output gitOutputFunc,
	runGates releaseGateRunner,
) (string, error) {
	commit, err := output("rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolving release commit: %w", err)
	}
	status, err := output("status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("checking release worktree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return "", errors.New("release gates require a clean worktree")
	}
	if err := runGates(commit); err != nil {
		return "", fmt.Errorf("release gates for commit %s: %w", commit, err)
	}
	afterGates, err := output("rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("verifying release commit after gates: %w", err)
	}
	if strings.TrimSpace(afterGates) != strings.TrimSpace(commit) {
		return "", fmt.Errorf("release commit changed while gates ran: started %s, now %s",
			commit, afterGates)
	}
	status, err = output("status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("verifying release worktree after gates: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return "", errors.New("release worktree changed while gates ran")
	}
	return strings.TrimSpace(commit), nil
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
		{name: "root audit", dir: root, args: []string{"mage", "audit"},
			lane: "root", exclusive: true},
		// Lint gates a release from GH-1479 on. It could not before: the policy
		// had never run, and its first run reported findings, which GH-1481
		// cleared.
		{name: "root lint", dir: root, args: []string{"mage", "lint"},
			lane: "root-lint", resources: []releaseResourceClass{releaseResourceCPU}},
		// DA_RELEASE_GATE makes the UI reproducibility gate treat a missing npm
		// as fatal rather than a developer skip (GH-1349), so a release cannot
		// pass without rebuilding shipped UIs and auditing their dependencies.
		{name: "root test", dir: root, args: []string{"mage", "test"},
			env:       []string{uiDistReleaseEnv + "=1"},
			lane:      "root-test",
			resources: []releaseResourceClass{releaseResourceCPU}},
		{name: "agent-core integration", dir: filepath.Join(root, "agent-core"),
			args: []string{"mage", "integration:all"}, lane: "agent-core",
			resources: []releaseResourceClass{releaseResourceCPU, releaseResourceHostOllama}},
		{name: "catalog integration", dir: catalogRoot,
			args: []string{"mage", "integration:all"}, lane: "catalog",
			resources: []releaseResourceClass{releaseResourceCPU}},
		{name: "catalog conformance", dir: catalogRoot,
			args: []string{"mage", "conformance"}, lane: "catalog",
			resources: []releaseResourceClass{releaseResourceCPU}},
	}
	// Every released application module must enter the release gate through its
	// own integration:all aggregate; otherwise an application is tagged without
	// its application-owned integration evidence ever running (GH-1343).
	for _, mod := range applicationModules {
		priority := 0
		resources := []releaseResourceClass{releaseResourceDocker}
		if mod == "applications/chatbot-mesh" {
			priority = chatbotReleasePriority
			resources = append(resources, releaseResourceHostOllama)
		}
		gates = append(gates, releaseGate{
			name:      mod + " integration",
			dir:       filepath.Join(root, filepath.FromSlash(mod)),
			args:      []string{"mage", "integration:all"},
			lane:      mod,
			resources: resources,
			priority:  priority,
		})
	}
	return gates
}

func executeReleaseGates(gates []releaseGate, run releaseCommandRunner) error {
	firstScheduled := 0
	for firstScheduled < len(gates) && gates[firstScheduled].exclusive {
		if err := executeReleaseSchedule(
			gates[firstScheduled:firstScheduled+1],
			firstScheduled,
			map[releaseResourceClass]int{},
			run,
		); err != nil {
			return err
		}
		firstScheduled++
	}
	return executeReleaseSchedule(
		gates[firstScheduled:],
		firstScheduled,
		releaseResourceCapacities,
		run,
	)
}

type releaseLane struct {
	index     int
	name      string
	gates     []indexedReleaseGate
	next      int
	active    bool
	started   bool
	startedAt time.Time
}

type indexedReleaseGate struct {
	index int
	gate  releaseGate
}

type releaseGateResult struct {
	index     int
	laneIndex int
	err       error
}

func executeReleaseSchedule(
	gates []releaseGate,
	indexOffset int,
	capacities map[releaseResourceClass]int,
	run releaseCommandRunner,
) error {
	lanes := releaseLanes(gates, indexOffset)
	if len(lanes) == 0 {
		return nil
	}
	if err := validateReleaseResources(gates, capacities); err != nil {
		return err
	}

	results := make(chan releaseGateResult, len(gates))
	inUse := make(map[releaseResourceClass]int)
	var failureDetected atomic.Bool
	active := 0
	launch := func(laneIndex int) {
		lane := &lanes[laneIndex]
		item := lane.gates[lane.next]
		if !lane.started {
			lane.started = true
			lane.startedAt = time.Now()
			fmt.Printf("=== release lane: %s ===\n", lane.name)
		}
		acquireReleaseResources(item.gate.resources, inUse)
		lane.active = true
		active++
		go func() {
			err := executeReleaseGate(item.gate, run)
			if err != nil {
				// Publish failure before the result so the scheduler cannot
				// launch another gate while this completion is buffered behind
				// another in-flight result.
				failureDetected.Store(true)
			}
			results <- releaseGateResult{
				index:     item.index,
				laneIndex: laneIndex,
				err:       err,
			}
		}()
	}

	var failures []releaseGateResult
	for {
		if len(failures) == 0 && !failureDetected.Load() {
			for _, laneIndex := range runnableReleaseLanes(lanes) {
				if failureDetected.Load() {
					break
				}
				gate := lanes[laneIndex].gates[lanes[laneIndex].next].gate
				if releaseResourcesAvailable(gate.resources, capacities, inUse) {
					launch(laneIndex)
				}
			}
		}
		if active == 0 {
			break
		}

		result := <-results
		active--
		lane := &lanes[result.laneIndex]
		lane.active = false
		releaseReleaseResources(lane.gates[lane.next].gate.resources, inUse)
		if result.err != nil {
			failures = append(failures, result)
			continue
		}
		lane.next++
		if lane.next == len(lane.gates) {
			fmt.Printf("=== release lane complete: %s (%s) ===\n",
				lane.name, time.Since(lane.startedAt).Round(time.Millisecond))
		}
	}

	if len(failures) == 0 {
		for _, lane := range lanes {
			if lane.next != len(lane.gates) {
				gate := lane.gates[lane.next].gate
				return fmt.Errorf("release scheduler cannot launch %s with resources %v",
					gate.name, gate.resources)
			}
		}
		return nil
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].index < failures[j].index })
	return failures[0].err
}

func releaseLanes(gates []releaseGate, indexOffset int) []releaseLane {
	indexByName := make(map[string]int)
	var lanes []releaseLane
	for gateIndex, gate := range gates {
		laneName := gate.lane
		if laneName == "" {
			laneName = "serial"
		}
		index, ok := indexByName[laneName]
		if !ok {
			index = len(lanes)
			indexByName[laneName] = index
			lanes = append(lanes, releaseLane{index: index, name: laneName})
		}
		lanes[index].gates = append(lanes[index].gates, indexedReleaseGate{
			index: indexOffset + gateIndex,
			gate:  gate,
		})
	}
	return lanes
}

func runnableReleaseLanes(lanes []releaseLane) []int {
	var runnable []int
	for laneIndex := range lanes {
		lane := &lanes[laneIndex]
		if !lane.active && lane.next < len(lane.gates) {
			runnable = append(runnable, laneIndex)
		}
	}
	sort.SliceStable(runnable, func(i, j int) bool {
		left := lanes[runnable[i]].gates[lanes[runnable[i]].next]
		right := lanes[runnable[j]].gates[lanes[runnable[j]].next]
		if left.gate.priority != right.gate.priority {
			return left.gate.priority > right.gate.priority
		}
		return left.index < right.index
	})
	return runnable
}

func validateReleaseResources(
	gates []releaseGate,
	capacities map[releaseResourceClass]int,
) error {
	for _, gate := range gates {
		required := make(map[releaseResourceClass]int)
		for _, resource := range gate.resources {
			required[resource]++
			if required[resource] > capacities[resource] {
				return fmt.Errorf(
					"release gate %s requires %d %s slots, capacity is %d",
					gate.name, required[resource], resource, capacities[resource])
			}
		}
	}
	return nil
}

func releaseResourcesAvailable(
	resources []releaseResourceClass,
	capacities map[releaseResourceClass]int,
	inUse map[releaseResourceClass]int,
) bool {
	required := make(map[releaseResourceClass]int)
	for _, resource := range resources {
		required[resource]++
		if inUse[resource]+required[resource] > capacities[resource] {
			return false
		}
	}
	return true
}

func acquireReleaseResources(
	resources []releaseResourceClass,
	inUse map[releaseResourceClass]int,
) {
	for _, resource := range resources {
		inUse[resource]++
	}
}

func releaseReleaseResources(
	resources []releaseResourceClass,
	inUse map[releaseResourceClass]int,
) {
	for _, resource := range resources {
		inUse[resource]--
	}
}

func executeReleaseGate(gate releaseGate, run releaseCommandRunner) error {
	started := time.Now()
	fmt.Printf("=== release gate: %s ===\n", gate.name)
	if err := run(gate); err != nil {
		elapsed := time.Since(started).Round(time.Millisecond)
		logReleaseGatePhase(gate, elapsed, "failure")
		return fmt.Errorf("%s failed after %s: %w", gate.name, elapsed, err)
	}
	elapsed := time.Since(started).Round(time.Millisecond)
	fmt.Printf("=== release gate complete: %s (%s) ===\n", gate.name, elapsed)
	logReleaseGatePhase(gate, elapsed, "success")
	return nil
}

func logReleaseGatePhase(gate releaseGate, elapsed time.Duration, outcome string) {
	fmt.Printf("phase target=release name=%s elapsed=%s outcome=%s lane=%s\n",
		releasePhaseField(gate.name), elapsed, outcome, releasePhaseField(gate.lane))
}

func releasePhaseField(value string) string {
	return strings.NewReplacer(" ", "-", "/", "-").Replace(value)
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
