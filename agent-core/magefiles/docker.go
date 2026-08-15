// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	defaultContainerImage = "agent-core:latest"
	defaultContainerNetRC = ".netrc"
	defaultProfilesMount  = "/profiles"
	defaultWorkMount      = "/work"

	dockerEngine = "docker"
)

// Docker builds the Agent Core runtime image from the latest release tag.
func Docker() error {
	ref, err := containerReleaseRef()
	if err != nil {
		return err
	}
	opts, err := dockerBuildOptionsFromDemo(ref)
	if err != nil {
		return err
	}

	args := containerBuildArgs(opts)
	fmt.Print(containerBuildSummary(opts, args))
	cmd := exec.Command(dockerEngine, args...)
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type dockerBuildOptions struct {
	Image string
	Ref   string
	Repo  string
	NetRC string
}

func dockerBuildOptionsFromDemo(ref string) (dockerBuildOptions, error) {
	return resolveDockerBuildOptions(ref, ".", exec.LookPath)
}

func resolveDockerBuildOptions(ref, root string, lookPath lookPathFunc) (dockerBuildOptions, error) {
	if err := requireDocker(lookPath); err != nil {
		return dockerBuildOptions{}, err
	}
	config, err := loadAgentCoreDemoConfig(root)
	if err != nil {
		return dockerBuildOptions{}, err
	}
	return dockerBuildOptions{
		Image: valueOrDefault(config.ContainerImage, defaultContainerImage),
		Ref:   ref,
		Repo:  strings.TrimSpace(config.ReleaseRepo),
		NetRC: valueOrDefault(config.ContainerNetRC, defaultContainerNetRC),
	}, nil
}

type lookPathFunc func(string) (string, error)

func requireDocker(lookPath lookPathFunc) error {
	if _, err := lookPath(dockerEngine); err != nil {
		return fmt.Errorf("docker not found on PATH; install Docker to build the container image")
	}
	return nil
}

func containerBuildArgs(opts dockerBuildOptions) []string {
	args := []string{"build", "--progress=plain"}
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
	fmt.Fprintf(&b, "building %s from %s with %s\n", opts.Image, opts.Ref, dockerEngine)
	fmt.Fprintln(&b, "build settings:")
	fmt.Fprintf(&b, "  engine: %s\n", dockerEngine)
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
	fmt.Fprintln(&b, "  docker buildkit: enabled")
	fmt.Fprintln(&b, "  docker progress: plain")
	fmt.Fprintln(&b, "  container output: streamed directly")
	fmt.Fprintf(&b, "command: %s\n", displayBuildCommand(opts, args))
	fmt.Fprintf(&b, "mounted profile example: %s\n", displayRuntimeCommand(opts))
	return b.String()
}

func displayBuildCommand(opts dockerBuildOptions, args []string) string {
	cmd := append([]string{dockerEngine}, args...)
	cmd = append([]string{"DOCKER_BUILDKIT=1"}, cmd...)
	return shellCommand(cmd)
}

func displayRuntimeCommand(opts dockerBuildOptions) string {
	return shellCommand([]string{
		dockerEngine, "run", "--rm",
		"-v", "/path/to/applications/catalog:" + defaultProfilesMount + ":ro",
		"-v", "$PWD:" + defaultWorkMount,
		"-w", defaultWorkMount,
		opts.Image,
		"--profile", defaultProfilesMount + "/agents/executor/profile.yaml",
		"--directory", defaultWorkMount,
	})
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

func valueOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
