// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestContainerEngineUsesOverride(t *testing.T) {
	got, err := containerEngine("docker", func(name string) (string, error) {
		t.Fatalf("lookPath called for override %q", name)
		return "", nil
	})
	if err != nil {
		t.Fatalf("containerEngine returned error: %v", err)
	}
	if got != "docker" {
		t.Fatalf("containerEngine = %q, want docker", got)
	}
}

func TestContainerEnginePrefersPodman(t *testing.T) {
	got, err := containerEngine("", func(name string) (string, error) {
		if name == "podman" {
			return "/usr/bin/podman", nil
		}
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("containerEngine returned error: %v", err)
	}
	if got != "podman" {
		t.Fatalf("containerEngine = %q, want podman", got)
	}
}

func TestContainerEngineFallsBackToDocker(t *testing.T) {
	got, err := containerEngine("", func(name string) (string, error) {
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("containerEngine returned error: %v", err)
	}
	if got != "docker" {
		t.Fatalf("containerEngine = %q, want docker", got)
	}
}

func TestContainerEngineErrorsWhenMissing(t *testing.T) {
	_, err := containerEngine("", func(name string) (string, error) {
		return "", errors.New("missing")
	})
	if err == nil {
		t.Fatal("containerEngine returned nil error for missing engines")
	}
	if !strings.Contains(err.Error(), envContainerEngine) {
		t.Fatalf("error = %q, want mention %s", err, envContainerEngine)
	}
}

func TestContainerBuildArgsForPodman(t *testing.T) {
	got := containerBuildArgs(dockerBuildOptions{
		Engine:    "podman",
		Image:     "registry.example/agent-core:test",
		Ref:       "v0.20260612.2",
		Repo:      "https://example.invalid/agent-core.git",
		NetRC:     "/home/user/.netrc",
		TLSVerify: "false",
	})
	want := []string{
		"build",
		"--tls-verify=false",
		"--secret", "id=git_credentials,src=/home/user/.netrc",
		"--build-arg", "AGENT_CORE_REF=v0.20260612.2",
		"--build-arg", "AGENT_CORE_REPO=https://example.invalid/agent-core.git",
		"-t", "registry.example/agent-core:test",
		".",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("containerBuildArgs = %#v, want %#v", got, want)
	}
}

func TestContainerBuildArgsForDocker(t *testing.T) {
	got := containerBuildArgs(dockerBuildOptions{
		Engine: "docker",
		Image:  "agent-core:latest",
		Ref:    "v0.20260612.2",
	})
	want := []string{
		"build",
		"--build-arg", "AGENT_CORE_REF=v0.20260612.2",
		"-t", "agent-core:latest",
		".",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("containerBuildArgs = %#v, want %#v", got, want)
	}
}

func TestNetrcPathUsesOverride(t *testing.T) {
	got := netrcPath(" /tmp/custom.netrc ", "/home/user", func(path string) (os.FileInfo, error) {
		t.Fatalf("stat called for override %q", path)
		return nil, nil
	})
	if got != "/tmp/custom.netrc" {
		t.Fatalf("netrcPath = %q, want /tmp/custom.netrc", got)
	}
}

func TestNetrcPathUsesDefaultWhenPresent(t *testing.T) {
	got := netrcPath("", "/home/user", func(path string) (os.FileInfo, error) {
		if path != "/home/user/.netrc" {
			t.Fatalf("stat path = %q, want /home/user/.netrc", path)
		}
		return nil, nil
	})
	if got != "/home/user/.netrc" {
		t.Fatalf("netrcPath = %q, want /home/user/.netrc", got)
	}
}

func TestTLSVerifyForEngine(t *testing.T) {
	if got := tlsVerifyForEngine("podman", ""); got != "false" {
		t.Fatalf("tlsVerifyForEngine podman default = %q, want false", got)
	}
	if got := tlsVerifyForEngine("docker", ""); got != "" {
		t.Fatalf("tlsVerifyForEngine docker default = %q, want empty", got)
	}
	if got := tlsVerifyForEngine("podman", "true"); got != "true" {
		t.Fatalf("tlsVerifyForEngine override = %q, want true", got)
	}
}
