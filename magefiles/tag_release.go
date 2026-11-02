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
)

// releaseModule owns the release assets and the mage targets that publish and
// check them; this repository's tag is released from it (srd007 R5).
const releaseModule = "applications/scrum-master"

var releaseTagPattern = regexp.MustCompile(`^v0\.(\d{8})\.(\d+)$`)

type releaseModuleRunner func(args ...string) error

// runReleaseModule runs one of the release module's mage targets.
func runReleaseModule(args ...string) error {
	cmd := exec.Command("mage", args...)
	cmd.Dir = filepath.FromSlash(releaseModule)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Released refuses a release tag install.sh cannot install: one with no
// release, a draft release, or a release lacking an asset. It reads the forge
// and changes nothing (srd007 R5.1).
func (TAG) Released(tag string) error {
	return runReleaseModule("released", strings.TrimSpace(tag))
}

// Publish publishes a pushed release tag from a checkout at the tag, then
// proves install.sh can install it (srd007 R5.2). Run it after the push.
func (TAG) Publish(tag string) error {
	return publishReleaseTag(".", tag, runReleaseModule)
}

func publishReleaseTag(dir, tag string, run releaseModuleRunner) error {
	tag = strings.TrimSpace(tag)
	if err := verifyTagAnnotation(dir, tag); err != nil {
		return err
	}
	head, err := gitIn(dir, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("tag:publish %s: %w", tag, err)
	}
	commit, err := gitIn(dir, "rev-parse", tag+"^{commit}")
	if err != nil {
		return fmt.Errorf("tag:publish %s: %w", tag, err)
	}
	if strings.TrimSpace(head) != strings.TrimSpace(commit) {
		return fmt.Errorf("tag:publish %s: refused, HEAD is not at the tag and the release is built from HEAD; "+
			"run it from a checkout at the tag (R5.2)", tag)
	}
	if err := run("publish"); err != nil {
		return fmt.Errorf("tag:publish %s: mage publish in %s: %w", tag, releaseModule, err)
	}
	if err := run("released", tag); err != nil {
		return fmt.Errorf("tag:publish %s: %w", tag, err)
	}
	return nil
}

// requirePreviousReleasePublished refuses to start another release while the
// newest tag on origin cannot be installed, so a skipped publish stops the next
// release before its gates run (srd007 R5.3).
func requirePreviousReleasePublished(remoteTags func() (string, error), run releaseModuleRunner) error {
	tags, err := remoteTags()
	if err != nil {
		return fmt.Errorf("listing release tags on origin: %w", err)
	}
	tag := newestReleaseTag(tags)
	if tag == "" {
		return nil
	}
	if err := run("released", tag); err != nil {
		return fmt.Errorf("release: %s, the newest tag on origin, has no complete release: %w; "+
			"publish it from a checkout at the tag before cutting another (R5.3)", tag, err)
	}
	return nil
}

// newestReleaseTag is the highest date and revision among the tag lines; a tag
// of another shape, such as ui-kit/v1.0.0, is no release of this line.
func newestReleaseTag(lines string) string {
	newest, newestDate, newestRevision := "", -1, -1
	for _, line := range strings.Split(lines, "\n") {
		line = strings.TrimSpace(line)
		m := releaseTagPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		date, dateErr := strconv.Atoi(m[1])
		revision, revisionErr := strconv.Atoi(m[2])
		if dateErr != nil || revisionErr != nil {
			continue
		}
		if date > newestDate || (date == newestDate && revision > newestRevision) {
			newest, newestDate, newestRevision = line, date, revision
		}
	}
	return newest
}

func gitAllRemoteReleaseTags() (string, error) {
	out, err := exec.Command("git", "ls-remote", "--tags", "--refs", "origin", tagPrefix+"*").Output()
	if err != nil {
		return "", err
	}
	return parseRemoteTagRefs(string(out)), nil
}
