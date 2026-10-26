// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestGitlabRigSkipsWithoutToken(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "")
	if err := (Regression{}).Gitlab(); err != nil {
		t.Fatalf("regression:gitlab without a token: %v, want a skip", err)
	}
}

// mage regression stays offline: the GitLab rig is never one of its targets.
func TestGitlabRigIsNotARegressionTarget(t *testing.T) {
	for _, target := range integrationTargets {
		name := runtime.FuncForPC(reflect.ValueOf(target).Pointer()).Name()
		if strings.Contains(strings.ToLower(name), "gitlab") {
			t.Fatalf("%s is among the integration targets mage regression runs", name)
		}
	}
}

func TestGitlabWebBaseDropsTheAPIPath(t *testing.T) {
	t.Setenv("GITLAB_BASE_URL", "https://gitlab.example.com/api/v4")
	if got := gitlabWebBase(); got != "https://gitlab.example.com" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("GITLAB_BASE_URL", "")
	if got := gitlabWebBase(); got != "https://gitlab.com" {
		t.Fatalf("got %q", got)
	}
}
