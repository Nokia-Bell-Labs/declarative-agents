// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// recordingRunner records the release module's targets and fails the one named.
func recordingRunner(failing string) (*[]string, releaseModuleRunner) {
	calls := &[]string{}
	return calls, func(args ...string) error {
		call := strings.Join(args, " ")
		*calls = append(*calls, call)
		if failing != "" && strings.HasPrefix(call, failing) {
			return errors.New(failing + " failed")
		}
		return nil
	}
}

// R5.2: the tag at HEAD is published and then proved installable, in that order.
func TestTagPublishPublishesThenChecks(t *testing.T) {
	dir := scratchRepo(t)
	runGitInDir(t, dir, "tag", "-a", "v0.20261102.0", "-m", goodSummary)
	calls, run := recordingRunner("")
	if err := publishReleaseTag(dir, "v0.20261102.0", run); err != nil {
		t.Fatal(err)
	}
	if want := []string{"publish", "released v0.20261102.0"}; !slices.Equal(*calls, want) {
		t.Fatalf("ran %v, want %v", *calls, want)
	}
}

// R5.2: a tag verify refuses, or one away from HEAD, publishes nothing.
func TestTagPublishRefusesBeforePublishing(t *testing.T) {
	lightweight := scratchRepo(t)
	runGitInDir(t, lightweight, "tag", "v0.20261102.0")

	moved := scratchRepo(t)
	runGitInDir(t, moved, "tag", "-a", "v0.20261102.0", "-m", goodSummary)
	runGitInDir(t, moved, "commit", "--allow-empty", "-m", "after the tag")

	for name, c := range map[string]struct{ dir, refusal string }{
		"lightweight":    {lightweight, "lightweight"},
		"not at the tag": {moved, "HEAD is not at the tag"},
	} {
		t.Run(name, func(t *testing.T) {
			calls, run := recordingRunner("")
			err := publishReleaseTag(c.dir, "v0.20261102.0", run)
			if err == nil || !strings.Contains(err.Error(), c.refusal) {
				t.Fatalf("got %v, want a refusal naming %q", err, c.refusal)
			}
			if len(*calls) != 0 {
				t.Fatalf("ran %v after refusing", *calls)
			}
		})
	}
}

// R5.2: a failed publish is not reported as a release, and a publish that
// leaves the release incomplete fails the target.
func TestTagPublishFailsWhenTheReleaseIsNotComplete(t *testing.T) {
	dir := scratchRepo(t)
	runGitInDir(t, dir, "tag", "-a", "v0.20261102.0", "-m", goodSummary)
	for failing, want := range map[string][]string{
		"publish":  {"publish"},
		"released": {"publish", "released v0.20261102.0"},
	} {
		calls, run := recordingRunner(failing)
		err := publishReleaseTag(dir, "v0.20261102.0", run)
		if err == nil || !strings.Contains(err.Error(), failing+" failed") {
			t.Fatalf("%s: got %v", failing, err)
		}
		if !slices.Equal(*calls, want) {
			t.Fatalf("%s: ran %v, want %v", failing, *calls, want)
		}
	}
}

// R5.3: the newest tag on origin must be installable before another release
// starts, and a repository with no release tags has nothing to check.
func TestRequirePreviousReleasePublished(t *testing.T) {
	tags := func(lines string, err error) func() (string, error) {
		return func() (string, error) { return lines, err }
	}
	const pushed = "v0.20261028.4\nv0.20261101.9\nv0.20261101.10\nui-kit/v1.0.0"

	calls, run := recordingRunner("")
	if err := requirePreviousReleasePublished(tags(pushed, nil), run); err != nil {
		t.Fatal(err)
	}
	if want := []string{"released v0.20261101.10"}; !slices.Equal(*calls, want) {
		t.Fatalf("ran %v, want %v", *calls, want)
	}

	_, run = recordingRunner("released")
	err := requirePreviousReleasePublished(tags(pushed, nil), run)
	if err == nil || !strings.Contains(err.Error(), "v0.20261101.10") || !strings.Contains(err.Error(), "R5.3") {
		t.Fatalf("unpublished tag: %v", err)
	}

	calls, run = recordingRunner("")
	if err := requirePreviousReleasePublished(tags("ui-kit/v1.0.0\n", nil), run); err != nil || len(*calls) != 0 {
		t.Fatalf("no release tags: %v, ran %v", err, *calls)
	}

	_, run = recordingRunner("")
	err = requirePreviousReleasePublished(tags("", errors.New("no network")), run)
	if err == nil || !strings.Contains(err.Error(), "no network") {
		t.Fatalf("listing failure: %v", err)
	}
}

func TestNewestReleaseTagComparesNumerically(t *testing.T) {
	for lines, want := range map[string]string{
		"v0.20261101.9\nv0.20261101.10": "v0.20261101.10",
		"v0.20261231.5\nv0.20270101.0":  "v0.20270101.0",
		"ui-kit/v1.0.0\nv0.1.0":         "",
		"":                              "",
	} {
		if got := newestReleaseTag(lines); got != want {
			t.Errorf("newestReleaseTag(%q) = %q, want %q", lines, got, want)
		}
	}
}
