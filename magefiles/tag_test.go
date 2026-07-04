// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCreateReleaseTagCreatesNextDailyTag(t *testing.T) {
	var calls [][]string
	err := createReleaseTag(
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
		func(args ...string) (string, error) {
			calls = append(calls, append([]string(nil), args...))
			switch strings.Join(args, " ") {
			case "rev-parse --abbrev-ref HEAD":
				return "main", nil
			case "tag -l v0.20260617.*":
				return strings.Join([]string{
					"v0.20260617.0",
					"v0.20260617.2",
					"v0.20260616.9",
					"not-a-release",
				}, "\n"), nil
			default:
				t.Fatalf("unexpected git output args: %q", strings.Join(args, " "))
				return "", nil
			}
		},
		func(args ...string) error {
			calls = append(calls, append([]string(nil), args...))
			return nil
		},
	)
	if err != nil {
		t.Fatalf("createReleaseTag returned error: %v", err)
	}
	want := [][]string{
		{"rev-parse", "--abbrev-ref", "HEAD"},
		{"tag", "-l", "v0.20260617.*"},
		{"tag", "v0.20260617.3"},
		{"tag", "agent-core/v0.20260617.3"},
		{"tag", "agent-profiles/v0.20260617.3"},
		{"tag", "design-patterns/v0.20260617.3"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("git calls = %#v, want %#v", calls, want)
	}
}

func TestCreateReleaseTagInGitRepository(t *testing.T) {
	root := initGitRepo(t)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp repo: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	date := "20260617"
	runGit(t, "tag", tagPrefix+date+".0")
	err = createReleaseTag(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC), gitOutput, gitExec)
	if err != nil {
		t.Fatalf("createReleaseTag returned error: %v", err)
	}
	out := runGitOutput(t, "tag", "-l", tagPrefix+date+".*")
	if !strings.Contains(out, tagPrefix+date+".1") {
		t.Fatalf("local tags = %q, want next daily revision", out)
	}
	for _, mod := range subModules {
		moduleTag := mod + "/" + tagPrefix + date + ".1"
		if !strings.Contains(runGitOutput(t, "tag", "-l", moduleTag), moduleTag) {
			t.Fatalf("local tags missing module tag %q", moduleTag)
		}
	}
}

func TestCreateReleaseTagRejectsNonMainBranch(t *testing.T) {
	err := createReleaseTag(time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		func(args ...string) (string, error) {
			return "feature/profile-tags", nil
		},
		func(args ...string) error {
			t.Fatalf("git exec called on non-main branch: %q", strings.Join(args, " "))
			return nil
		},
	)
	if err == nil {
		t.Fatal("createReleaseTag returned nil error for non-main branch")
	}
	if !strings.Contains(err.Error(), "tag must be run from main") {
		t.Fatalf("error = %q, want branch validation message", err)
	}
}

func TestCreateReleaseTagWrapsTagListingError(t *testing.T) {
	want := errors.New("git tag failed")
	err := createReleaseTag(time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		func(args ...string) (string, error) {
			if strings.Join(args, " ") == "rev-parse --abbrev-ref HEAD" {
				return "main", nil
			}
			return "", want
		},
		func(args ...string) error {
			t.Fatalf("git exec called after listing failure: %q", strings.Join(args, " "))
			return nil
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want to wrap %v", err, want)
	}
}

func TestCreateReleaseTagWrapsModuleTagFailure(t *testing.T) {
	want := errors.New("tag exists")
	err := createReleaseTag(time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		func(args ...string) (string, error) {
			switch strings.Join(args, " ") {
			case "rev-parse --abbrev-ref HEAD":
				return "main", nil
			case "tag -l v0.20260617.*":
				return "", nil
			default:
				t.Fatalf("unexpected git output args: %q", strings.Join(args, " "))
				return "", nil
			}
		},
		func(args ...string) error {
			if strings.Join(args, " ") == "tag agent-core/v0.20260617.0" {
				return want
			}
			return nil
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want to wrap %v", err, want)
	}
	if got := err.Error(); !strings.Contains(got, "agent-core/v0.20260617.0") || !strings.Contains(got, "module agent-core") {
		t.Fatalf("error = %q, want module tag context", got)
	}
}

func TestReleaseTags(t *testing.T) {
	got := releaseTags("v0.20260617.0", []string{"agent-core", "agent-profiles"})
	want := []string{"v0.20260617.0", "agent-core/v0.20260617.0", "agent-profiles/v0.20260617.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("releaseTags = %#v, want %#v", got, want)
	}
}

func TestNextRevisionFromTags(t *testing.T) {
	got := nextRevisionFromTags("20260617", strings.Join([]string{
		"v0.20260617.4",
		"v0.20260617.12",
		"v0.20260617.bad",
		"v0.20260616.99",
		"v1.20260617.20",
	}, "\n"))
	if got != 13 {
		t.Fatalf("nextRevisionFromTags = %d, want 13", got)
	}
}

func TestNextRevisionFromTagsStartsAtZero(t *testing.T) {
	got := nextRevisionFromTags("20260617", "v0.20260616.1\nnot-a-release")
	if got != 0 {
		t.Fatalf("nextRevisionFromTags empty day = %d, want 0", got)
	}
}

func TestValidateReleaseBranch(t *testing.T) {
	if err := validateReleaseBranch(" main\n"); err != nil {
		t.Fatalf("validateReleaseBranch main returned error: %v", err)
	}
	err := validateReleaseBranch("develop")
	if err == nil {
		t.Fatal("validateReleaseBranch returned nil error for develop")
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitInDir(t, root, "init", "-b", "main")
	if err := os.WriteFile(root+"/README.md", []byte("# temp\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGitInDir(t, root, "add", "README.md")
	runGitInDir(t, root, "-c", "user.name=Test User", "-c", "user.email=test@example.invalid", "commit", "-m", "init")
	return root
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	if err := exec.Command("git", args...).Run(); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func runGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func runGitOutput(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}
