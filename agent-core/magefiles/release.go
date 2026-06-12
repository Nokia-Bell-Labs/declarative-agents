// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	tagPrefix          = "v0."
	baseBranch         = "main"
	agentCoreRefEnvVar = "AGENT_CORE_REF"
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

// containerReleaseRef returns the release ref used for container builds.
func containerReleaseRef() (string, error) {
	return resolveContainerReleaseRef(os.Getenv(agentCoreRefEnvVar), gitOutput)
}

type gitOutputFunc func(args ...string) (string, error)

func resolveContainerReleaseRef(override string, git gitOutputFunc) (string, error) {
	if ref := strings.TrimSpace(override); ref != "" {
		return ref, nil
	}

	out, err := git("tag", "--list", tagPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("list release tags: %w", err)
	}
	tag, ok := latestReleaseTag(strings.Split(out, "\n"))
	if !ok {
		return "", fmt.Errorf("no release tags matching %sYYYYMMDD.N", tagPrefix)
	}
	return tag, nil
}

func latestReleaseTag(tags []string) (string, bool) {
	releaseRe := regexp.MustCompile(`^` + regexp.QuoteMeta(tagPrefix) + `(\d{8})\.(\d+)$`)
	var latest string
	latestDate := ""
	latestRev := -1
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		m := releaseRe.FindStringSubmatch(tag)
		if len(m) != 3 {
			continue
		}
		rev, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if m[1] > latestDate || (m[1] == latestDate && rev > latestRev) {
			latest = tag
			latestDate = m[1]
			latestRev = rev
		}
	}
	return latest, latest != ""
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
