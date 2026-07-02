// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	tagPrefix  = "v0."
	baseBranch = "main"
)

type gitOutputFunc func(args ...string) (string, error)
type gitExecFunc func(args ...string) error

// Tag creates an agent-profiles release tag (v0.YYYYMMDD.N).
func Tag() error {
	return createReleaseTag(time.Now(), gitOutput, gitExec)
}

func createReleaseTag(now time.Time, output gitOutputFunc, run gitExecFunc) error {
	branch, err := output("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if err := validateReleaseBranch(branch); err != nil {
		return err
	}

	date := now.Format("20060102")
	tags, err := output("tag", "-l", tagPrefix+date+".*")
	if err != nil {
		return fmt.Errorf("listing local release tags: %w", err)
	}
	tag := fmt.Sprintf("%s%s.%d", tagPrefix, date, nextRevisionFromTags(date, tags))

	fmt.Printf("creating tag %s\n", tag)
	if err := run("tag", tag); err != nil {
		return fmt.Errorf("creating tag %s: %w", tag, err)
	}
	fmt.Printf("done - created %s\n", tag)
	return nil
}

func validateReleaseBranch(branch string) error {
	current := strings.TrimSpace(branch)
	if current != baseBranch {
		return fmt.Errorf("tag must be run from %s (currently on %s)", baseBranch, current)
	}
	return nil
}

func nextRevisionFromTags(date, tags string) int {
	revRe := regexp.MustCompile(`^` + regexp.QuoteMeta(tagPrefix) + regexp.QuoteMeta(date) + `\.(\d+)$`)
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

func gitExec(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
