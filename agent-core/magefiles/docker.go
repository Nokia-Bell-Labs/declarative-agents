// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	defaultContainerImage   = "agent-core:latest"
	defaultIntegrationImage = "agent-core-integration:latest"
	defaultContainerNetRC   = ".netrc"
	defaultProfilesMount    = "/profiles/agents"
	defaultWorkMount        = "/work"

	envContainerImage = "AGENT_CORE_IMAGE"
	envContainerNetRC = "AGENT_CORE_NETRC"

	dockerEngine = "docker"
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

func dockerBuildOptionsFromEnv(ref string) (dockerBuildOptions, error) {
	if err := requireDocker(exec.LookPath); err != nil {
		return dockerBuildOptions{}, err
	}
	return dockerBuildOptions{
		Image: envOrDefault(envContainerImage, defaultContainerImage),
		Ref:   ref,
		Repo:  strings.TrimSpace(os.Getenv(agentCoreRepoEnvVar)),
		NetRC: envOrDefault(envContainerNetRC, defaultContainerNetRC),
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
	fmt.Fprintf(&b, "integration image command: %s\n", displayIntegrationBuildCommand(opts))
	fmt.Fprintf(&b, "integration container example: %s\n", displayIntegrationCommand(opts))
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
		"-v", "/path/to/agent-profiles:" + defaultProfilesMount + ":ro",
		"-v", "$PWD:" + defaultWorkMount,
		"-w", defaultWorkMount,
		opts.Image,
		"--profile", defaultProfilesMount + "/agents/generator/profile.yaml",
		"--directory", defaultWorkMount,
	})
}

func displayIntegrationBuildCommand(opts dockerBuildOptions) string {
	args := containerBuildArgs(opts)
	for i, arg := range args {
		if arg == opts.Image && i > 0 && args[i-1] == "-t" {
			args[i] = defaultIntegrationImage
			break
		}
	}
	if len(args) > 0 {
		args = append(args[:len(args)-1], "--target", "integration", args[len(args)-1])
	}
	return displayBuildCommand(opts, args)
}

func displayIntegrationCommand(opts dockerBuildOptions) string {
	return shellCommand([]string{
		dockerEngine, "run", "--rm",
		"-v", "/path/to/agent-profiles:" + defaultProfilesMount + ":ro",
		"-w", "/src",
		defaultIntegrationImage,
		"mage", "integration:uc001",
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

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
