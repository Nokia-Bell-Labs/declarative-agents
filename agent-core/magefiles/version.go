// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"strings"
)

const versionPackagePath = "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/version"

type versionMeta struct {
	Version string
	Commit  string
	Date    string
}

// versionLdflags returns linker flags that populate internal/version.
// Git failures yield empty flags so a tarball build keeps the package
// defaults; metadata must not fail the compile.
func versionLdflags() (string, error) {
	return versionLdflagsFrom(gitOutput)
}

func versionLdflagsFrom(git gitOutputFunc) (string, error) {
	meta, ok := versionMetadataFrom(git)
	if !ok {
		return "", nil
	}
	return versionLdflagsString(meta), nil
}

func versionMetadataFrom(git gitOutputFunc) (versionMeta, bool) {
	version, err := git("describe", "--tags", "--always", "--dirty")
	if err != nil {
		return versionMeta{}, false
	}
	commit, err := git("rev-parse", "--short", "HEAD")
	if err != nil {
		return versionMeta{}, false
	}
	date, err := git("log", "-1", "--format=%cI")
	if err != nil {
		return versionMeta{}, false
	}
	return versionMeta{
		Version: strings.TrimSpace(version),
		Commit:  strings.TrimSpace(commit),
		Date:    strings.TrimSpace(date),
	}, true
}

func versionLdflagsString(meta versionMeta) string {
	p := versionPackagePath
	return fmt.Sprintf("-X %s.Version=%s -X %s.Commit=%s -X %s.Date=%s",
		p, meta.Version, p, meta.Commit, p, meta.Date)
}

func appendLdflags(args []string, ldflags string) []string {
	if ldflags == "" {
		return args
	}
	return append(args, "-ldflags", ldflags)
}

// withContainerVersion sets AGENT_* build-arg values from the image's
// release ref. Version is that ref, not git describe, so a tagged clone
// does not inherit the host checkout's describe output.
func withContainerVersion(opts dockerBuildOptions, git gitOutputFunc) dockerBuildOptions {
	opts.Version = opts.Ref
	if commit, err := git("rev-parse", "--short", opts.Ref); err == nil {
		opts.Commit = strings.TrimSpace(commit)
	}
	if date, err := git("log", "-1", "--format=%cI", opts.Ref); err == nil {
		opts.Date = strings.TrimSpace(date)
	}
	return opts
}

func containerVersionBuildArgs(opts dockerBuildOptions) []string {
	var args []string
	if opts.Version != "" {
		args = append(args, "--build-arg", "AGENT_VERSION="+opts.Version)
	}
	if opts.Commit != "" {
		args = append(args, "--build-arg", "AGENT_COMMIT="+opts.Commit)
	}
	if opts.Date != "" {
		args = append(args, "--build-arg", "AGENT_DATE="+opts.Date)
	}
	return args
}
