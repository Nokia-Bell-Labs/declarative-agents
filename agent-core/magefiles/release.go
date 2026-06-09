// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

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

// Tag creates a documentation release tag (v0.YYYYMMDD.N).
func Tag() error {
	branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if branch != baseBranch {
		return fmt.Errorf("tag must be run from %s (currently on %s)", baseBranch, branch)
	}

	today := time.Now().Format("20060102")
	rev := nextRevision(today)
	tag := fmt.Sprintf("%s%s.%d", tagPrefix, today, rev)

	fmt.Printf("creating tag %s\n", tag)
	if err := gitExec("tag", tag); err != nil {
		return fmt.Errorf("creating tag %s: %w", tag, err)
	}
	fmt.Printf("done — created %s\n", tag)
	return nil
}

// nextRevision finds the next revision number for today's tags.
func nextRevision(date string) int {
	pattern := fmt.Sprintf("%s%s.*", tagPrefix, date)
	out, err := gitOutput("tag", "-l", pattern)
	if err != nil || out == "" {
		return 0
	}

	revRe := regexp.MustCompile(`^` + regexp.QuoteMeta(tagPrefix) + regexp.QuoteMeta(date) + `\.(\d+)$`)
	maxRev := -1
	for _, line := range strings.Split(out, "\n") {
		m := revRe.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) == 2 {
			if rev, err := strconv.Atoi(m[1]); err == nil && rev > maxRev {
				maxRev = rev
			}
		}
	}
	if maxRev < 0 {
		return 0
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
