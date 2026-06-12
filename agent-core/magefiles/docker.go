// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	defaultContainerImage = "agent-core:latest"

	envContainerEngine = "AGENT_CORE_CONTAINER_ENGINE"
	envContainerImage  = "AGENT_CORE_IMAGE"
	envContainerNetRC  = "AGENT_CORE_NETRC"
	envTLSVerify       = "AGENT_CORE_TLS_VERIFY"
)

// Docker builds the Agent Core runtime image from the latest release tag.
func Docker() error {
	ref, err := containerReleaseRef()
	if err != nil {
		return err
	}
	opts, err := dockerBuildOptionsFromEnv(ref)
	if err != nil {
		return err
	}

	args := containerBuildArgs(opts)
	fmt.Print(containerBuildSummary(opts, args))
	cmd := exec.Command(opts.Engine, args...)
	if opts.Engine == "docker" {
		cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type dockerBuildOptions struct {
	Engine    string
	Image     string
	Ref       string
	Repo      string
	NetRC     string
	TLSVerify string
}

func dockerBuildOptionsFromEnv(ref string) (dockerBuildOptions, error) {
	engine, err := containerEngine(os.Getenv(envContainerEngine), exec.LookPath)
	if err != nil {
		return dockerBuildOptions{}, err
	}
	return dockerBuildOptions{
		Engine:    engine,
		Image:     envOrDefault(envContainerImage, defaultContainerImage),
		Ref:       ref,
		Repo:      strings.TrimSpace(os.Getenv(agentCoreRepoEnvVar)),
		NetRC:     netrcPath(os.Getenv(envContainerNetRC), os.Getenv("HOME"), os.Stat),
		TLSVerify: tlsVerifyForEngine(engine, os.Getenv(envTLSVerify)),
	}, nil
}

type lookPathFunc func(string) (string, error)

func containerEngine(override string, lookPath lookPathFunc) (string, error) {
	if engine := strings.TrimSpace(override); engine != "" {
		return engine, nil
	}
	if _, err := lookPath("podman"); err == nil {
		return "podman", nil
	}
	if _, err := lookPath("docker"); err == nil {
		return "docker", nil
	}
	return "", fmt.Errorf("no container engine found; set %s to podman or docker", envContainerEngine)
}

func containerBuildArgs(opts dockerBuildOptions) []string {
	args := []string{"build"}
	if opts.Engine == "podman" && opts.TLSVerify != "" {
		args = append(args, "--tls-verify="+opts.TLSVerify)
	}
	if opts.Engine == "docker" {
		args = append(args, "--progress=plain")
	}
	if opts.NetRC != "" {
		args = append(args, "--secret", "id=git_credentials,src="+opts.NetRC)
	}
	args = append(args,
		"--build-arg", "AGENT_CORE_REF="+opts.Ref,
	)
	if opts.Repo != "" {
		args = append(args, "--build-arg", "AGENT_CORE_REPO="+opts.Repo)
	}
	args = append(args, "-t", opts.Image, ".")
	return args
}

func containerBuildSummary(opts dockerBuildOptions, args []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "building %s from %s with %s\n", opts.Image, opts.Ref, opts.Engine)
	fmt.Fprintln(&b, "build settings:")
	fmt.Fprintf(&b, "  engine: %s\n", opts.Engine)
	fmt.Fprintf(&b, "  image: %s\n", opts.Image)
	fmt.Fprintf(&b, "  release ref: %s\n", opts.Ref)
	if opts.Repo != "" {
		fmt.Fprintf(&b, "  source repo: %s\n", opts.Repo)
	} else {
		fmt.Fprintf(&b, "  source repo: %s (Dockerfile default)\n", defaultAgentCoreRepo)
	}
	if opts.NetRC != "" {
		fmt.Fprintf(&b, "  git credentials secret: %s\n", opts.NetRC)
	} else {
		fmt.Fprintln(&b, "  git credentials secret: (none)")
	}
	if opts.Engine == "podman" {
		fmt.Fprintf(&b, "  podman tls verify: %s\n", opts.TLSVerify)
	}
	if opts.Engine == "docker" {
		fmt.Fprintln(&b, "  docker buildkit: enabled")
		fmt.Fprintln(&b, "  docker progress: plain")
	}
	fmt.Fprintln(&b, "  container output: streamed directly")
	fmt.Fprintf(&b, "command: %s\n", displayBuildCommand(opts, args))
	return b.String()
}

func displayBuildCommand(opts dockerBuildOptions, args []string) string {
	cmd := append([]string{opts.Engine}, args...)
	if opts.Engine == "docker" {
		cmd = append([]string{"DOCKER_BUILDKIT=1"}, cmd...)
	}
	return shellCommand(cmd)
}

func shellCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.IndexFunc(arg, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			strings.ContainsRune("@%_+=:,./-", r))
	}) == -1 {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

type statFunc func(string) (os.FileInfo, error)

func netrcPath(override, home string, stat statFunc) string {
	if path := strings.TrimSpace(override); path != "" {
		return path
	}
	if home == "" {
		return ""
	}
	path := filepath.Join(home, ".netrc")
	if _, err := stat(path); err == nil {
		return path
	}
	return ""
}

func tlsVerifyForEngine(engine, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if engine == "podman" {
		return "false"
	}
	return ""
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
