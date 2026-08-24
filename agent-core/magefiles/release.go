// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	tagPrefix            = "v0."
	agentCoreDemoFile    = "demo.yaml"
	defaultAgentCoreRepo = "https://github.com/Nokia-Bell-Labs/declarative-agents/agent-core.git"
)

type agentCoreDemoConfig struct {
	ReleaseRef     string `yaml:"release_ref"`
	ReleaseRepo    string `yaml:"release_repo"`
	ContainerImage string `yaml:"container_image"`
	ContainerNetRC string `yaml:"container_netrc"`
	DoltBin        string `yaml:"dolt_bin"`
}

// containerReleaseRef returns the release ref used for container builds.
// mage docker passes this ref as AGENT_VERSION so the image reports the
// tag it cloned, not git describe from the build-host checkout.
func containerReleaseRef() (string, error) {
	return containerReleaseRefFrom(".", gitOutput)
}

func containerReleaseRefFrom(root string, git gitOutputFunc) (string, error) {
	config, err := loadAgentCoreDemoConfig(root)
	if err != nil {
		return "", err
	}
	return resolveContainerReleaseRef(config.ReleaseRef, config.ReleaseRepo, git)
}

func loadAgentCoreDemoConfig(root string) (agentCoreDemoConfig, error) {
	configPath := filepath.Join(root, agentCoreDemoFile)
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return agentCoreDemoConfig{}, nil
	}
	if err != nil {
		return agentCoreDemoConfig{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	var config agentCoreDemoConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return agentCoreDemoConfig{}, fmt.Errorf("parse %s: %w", configPath, err)
	}
	return config, nil
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
