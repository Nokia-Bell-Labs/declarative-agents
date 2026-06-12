// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveContainerReleaseRefUsesOverride(t *testing.T) {
	called := false
	got, err := resolveContainerReleaseRef(" v0.20260612.2 ", func(args ...string) (string, error) {
		called = true
		return "", nil
	})
	if err != nil {
		t.Fatalf("resolveContainerReleaseRef override returned error: %v", err)
	}
	if got != "v0.20260612.2" {
		t.Fatalf("resolveContainerReleaseRef override = %q, want v0.20260612.2", got)
	}
	if called {
		t.Fatal("resolveContainerReleaseRef called git despite override")
	}
}

func TestResolveContainerReleaseRefFindsLatestReleaseTag(t *testing.T) {
	got, err := resolveContainerReleaseRef("", func(args ...string) (string, error) {
		if strings.Join(args, " ") != "tag --list v0.*" {
			t.Fatalf("git args = %q, want tag --list v0.*", strings.Join(args, " "))
		}
		return strings.Join([]string{
			"v0.20260611.4",
			"not-a-release",
			"v0.20260612.1",
			"v0.20260612.10",
			"v0.20260612.bad",
			"v0.20260609.99",
		}, "\n"), nil
	})
	if err != nil {
		t.Fatalf("resolveContainerReleaseRef returned error: %v", err)
	}
	if got != "v0.20260612.10" {
		t.Fatalf("resolveContainerReleaseRef = %q, want v0.20260612.10", got)
	}
}

func TestResolveContainerReleaseRefErrorsWhenNoReleaseTags(t *testing.T) {
	_, err := resolveContainerReleaseRef("", func(args ...string) (string, error) {
		return "v1.0.0\njunk\nv0.20260612", nil
	})
	if err == nil {
		t.Fatal("resolveContainerReleaseRef returned nil error for no release tags")
	}
	if !strings.Contains(err.Error(), "no release tags") {
		t.Fatalf("error = %q, want no release tags", err)
	}
}

func TestResolveContainerReleaseRefWrapsGitError(t *testing.T) {
	want := errors.New("git unavailable")
	_, err := resolveContainerReleaseRef("", func(args ...string) (string, error) {
		return "", want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want to wrap %v", err, want)
	}
}

func TestLatestReleaseTag(t *testing.T) {
	got, ok := latestReleaseTag([]string{
		"v0.20260608.4",
		"v0.20260608.12",
		"v0.20260609.0",
	})
	if !ok {
		t.Fatal("latestReleaseTag returned ok=false")
	}
	if got != "v0.20260609.0" {
		t.Fatalf("latestReleaseTag = %q, want v0.20260609.0", got)
	}
}
