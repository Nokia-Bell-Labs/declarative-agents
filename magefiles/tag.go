// Copyright (c) 2026 Nokia. All rights reserved.

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
	tagPrefix  = "v0."
	baseBranch = "main"
)

// Tag creates a repository-wide release tag and matching module tags.
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

	for _, releaseTag := range releaseTags(tag, subModules) {
		fmt.Printf("creating tag %s\n", releaseTag)
		if err := run("tag", releaseTag); err != nil {
			return releaseTagError(releaseTag, err)
		}
	}
	fmt.Printf("done — created %s\n", strings.Join(releaseTags(tag, subModules), ", "))
	return nil
}

func releaseTags(rootTag string, modules []string) []string {
	tags := []string{rootTag}
	for _, mod := range modules {
		tags = append(tags, mod+"/"+rootTag)
	}
	return tags
}

func releaseTagError(tag string, err error) error {
	module, _, ok := strings.Cut(tag, "/")
	if ok {
		return fmt.Errorf("creating tag %s for module %s: %w", tag, module, err)
	}
	return fmt.Errorf("creating tag %s: %w", tag, err)
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

type gitOutputFunc func(args ...string) (string, error)
type gitExecFunc func(args ...string) error

func gitExec(args ...string) error {
	cmd := exec.Command("git", args...)
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
