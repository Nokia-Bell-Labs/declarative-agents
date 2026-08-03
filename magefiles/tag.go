// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	tagPrefix              = "v0."
	baseBranch             = "main"
	catalogModule          = "applications/catalog"
	legacyCatalogTagPrefix = "agent-profiles/"
)

type releaseGate struct {
	name string
	dir  string
	args []string
	env  []string
}

type releaseGateRunner func(string) error
type releaseCommandRunner func(releaseGate) error

type remoteTagsFunc func(date string) (string, error)

// Tag creates a repository-wide release tag and matching module tags.
func Tag() error {
	return createReleaseTag(time.Now(), gitOutput, gitRemoteTags, gitCreateTagSet, runReleaseGates)
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

	allTags := releaseTags(tag, releaseModules())
	fmt.Printf("creating atomic tag set %s\n", strings.Join(allTags, ", "))
	if err := createTags(allTags, commit); err != nil {
		return fmt.Errorf("creating atomic release tag set: %w", err)
	}
	fmt.Printf("done — created %s\n", strings.Join(allTags, ", "))
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
	return []releaseGate{
		{name: "root audit", dir: root, args: []string{"mage", "audit"}},
		{name: "root test", dir: root, args: []string{"mage", "test"}},
		{name: "agent-core integration", dir: filepath.Join(root, "agent-core"),
			args: []string{"mage", "integration:all"}},
		{name: "catalog integration", dir: catalogRoot,
			args: []string{"mage", "integration:all"}},
		{name: "catalog conformance", dir: catalogRoot,
			args: []string{"mage", "conformance"}},
	}
}

func executeReleaseGates(gates []releaseGate, run releaseCommandRunner) error {
	for _, gate := range gates {
		fmt.Printf("=== release gate: %s ===\n", gate.name)
		if err := run(gate); err != nil {
			return fmt.Errorf("%s failed: %w", gate.name, err)
		}
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

func releaseTags(rootTag string, modules []string) []string {
	tags := []string{rootTag}
	for _, mod := range modules {
		tags = append(tags, mod+"/"+rootTag)
		if mod == catalogModule && strings.HasPrefix(rootTag, tagPrefix) {
			// Release 99 keeps the historical v0 module identifier executable.
			// Both names are created atomically at the same immutable commit.
			tags = append(tags, legacyCatalogTagPrefix+rootTag)
		}
	}
	return tags
}

func releaseModules() []string {
	return append(append([]string{}, subModules...), applicationModules...)
}

func validateReleaseBranch(branch string) error {
	current := strings.TrimSpace(branch)
	if current != baseBranch {
		return fmt.Errorf("tag must be run from %s (currently on %s)", baseBranch, current)
	}
	return nil
}

func nextRevisionFromTags(date, tags string) int {
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
