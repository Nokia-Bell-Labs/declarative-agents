// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const (
	tagPrefix            = "v0."
	agentCoreRefEnvVar   = "AGENT_CORE_REF"
	agentCoreRepoEnvVar  = "AGENT_CORE_REPO"
	defaultAgentCoreRepo = "https://github.com/Nokia-Bell-Labs/declarative-agents/agent-core.git"
)

// containerReleaseRef returns the release ref used for container builds.
func containerReleaseRef() (string, error) {
	return resolveContainerReleaseRef(os.Getenv(agentCoreRefEnvVar), os.Getenv(agentCoreRepoEnvVar), gitOutput)
}

type gitOutputFunc func(args ...string) (string, error)

func resolveContainerReleaseRef(override, repoOverride string, git gitOutputFunc) (string, error) {
	if ref := strings.TrimSpace(override); ref != "" {
		return ref, nil
	}

	repo := strings.TrimSpace(repoOverride)
	if repo == "" {
		repo = defaultAgentCoreRepo
	}
	out, err := git("ls-remote", "--tags", "--refs", repo, tagPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("list remote release tags from %s: %w", repo, err)
	}
	tag, ok := latestReleaseTag(remoteReleaseTagNames(out))
	if !ok {
		return "", fmt.Errorf("no release tags matching %sYYYYMMDD.N", tagPrefix)
	}
	return tag, nil
}

func remoteReleaseTagNames(out string) []string {
	var tags []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if tag, ok := strings.CutPrefix(fields[1], "refs/tags/"); ok {
			tags = append(tags, tag)
		}
	}
	return tags
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
