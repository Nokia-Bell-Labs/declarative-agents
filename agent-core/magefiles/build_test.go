// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestAuditRunFailed(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "terminal state failed",
			output: `2026/06/16 20:57:05 run complete: status=failed iterations=1
terminal state: failed
`,
			want: true,
		},
		{
			name:   "run summary failed",
			output: "2026/06/16 20:57:05 run complete: status=failed iterations=1\n",
			want:   true,
		},
		{
			name:   "succeeded",
			output: "2026/06/16 20:57:05 run complete: status=succeeded iterations=1\nterminal state: succeeded\n",
			want:   false,
		},
		{
			name:   "empty",
			output: "",
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := auditRunFailed(tc.output)
			if got != tc.want {
				t.Fatalf("auditRunFailed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnvWithDefault(t *testing.T) {
	t.Parallel()

	got := envWithDefault([]string{"PATH=/bin"}, "TEST_OVERRIDE_PATH", "/repo")
	if !containsEnv(got, "TEST_OVERRIDE_PATH=/repo") {
		t.Fatalf("envWithDefault() = %v, want TEST_OVERRIDE_PATH default", got)
	}

	existing := []string{"TEST_OVERRIDE_PATH=/custom"}
	got = envWithDefault(existing, "TEST_OVERRIDE_PATH", "/repo")
	if len(got) != 1 || got[0] != "TEST_OVERRIDE_PATH=/custom" {
		t.Fatalf("envWithDefault() = %v, want existing value preserved", got)
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func TestVersionLdflagsFromInjectsModulePath(t *testing.T) {
	t.Parallel()
	git := func(args ...string) (string, error) {
		switch strings.Join(args, " ") {
		case "describe --tags --always --dirty":
			return "v0.20260824.1-3-gabc1234", nil
		case "rev-parse --short HEAD":
			return "abc1234", nil
		case "log -1 --format=%cI":
			return "2026-08-24T15:00:00-04:00", nil
		default:
			t.Fatalf("unexpected git args %q", args)
			return "", nil
		}
	}
	got, err := versionLdflagsFrom(git)
	if err != nil {
		t.Fatalf("versionLdflagsFrom returned error: %v", err)
	}
	want := "-X " + versionPackagePath + ".Version=v0.20260824.1-3-gabc1234 -X " +
		versionPackagePath + ".Commit=abc1234 -X " +
		versionPackagePath + ".Date=2026-08-24T15:00:00-04:00"
	if got != want {
		t.Fatalf("versionLdflagsFrom = %q, want %q", got, want)
	}
}

func TestVersionLdflagsFromSkipsWhenGitUnavailable(t *testing.T) {
	t.Parallel()
	got, err := versionLdflagsFrom(func(...string) (string, error) {
		return "", errors.New("git not found")
	})
	if err != nil {
		t.Fatalf("versionLdflagsFrom error = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("versionLdflagsFrom = %q, want empty flags", got)
	}
}

func TestVersionLdflagsStringIsDeterministic(t *testing.T) {
	t.Parallel()
	meta := versionMeta{
		Version: "v0.20260824.1",
		Commit:  "abc1234",
		Date:    "2026-08-24T15:00:00-04:00",
	}
	if versionLdflagsString(meta) != versionLdflagsString(meta) {
		t.Fatal("versionLdflagsString must be stable for the same commit metadata")
	}
}

func TestAppendLdflagsOmitsEmpty(t *testing.T) {
	t.Parallel()
	args := []string{"build", "-o", "bin/agent"}
	if got := appendLdflags(args, ""); !reflect.DeepEqual(got, args) {
		t.Fatalf("appendLdflags empty = %#v, want %#v", got, args)
	}
	got := appendLdflags(args, "-X example.Version=v1")
	want := []string{"build", "-o", "bin/agent", "-ldflags", "-X example.Version=v1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendLdflags = %#v, want %#v", got, want)
	}
}
