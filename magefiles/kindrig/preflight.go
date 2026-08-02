// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	MinimumDockerCPUs       = 4
	MinimumDockerMemoryGiB  = 6
	preflightCommandTimeout = 10 * time.Second
)

// HostRunner executes one non-mutating host preflight command.
type HostRunner func(context.Context, string, ...string) ([]byte, error)

type ToolRequirement struct {
	Name        string
	Minimum     Version
	VersionArgs []string
	InstallURL  string
}

type Version struct {
	Major int
	Minor int
	Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

var StandardToolchain = []ToolRequirement{
	{Name: "docker", Minimum: Version{24, 0, 0},
		VersionArgs: []string{"version", "--format", "{{.Client.Version}}"},
		InstallURL:  "https://docs.docker.com/engine/install/"},
	{Name: "kind", Minimum: Version{0, 32, 0},
		VersionArgs: []string{"version"},
		InstallURL:  "https://kind.sigs.k8s.io/docs/user/quick-start/#installation"},
	{Name: "helm", Minimum: Version{3, 17, 0},
		VersionArgs: []string{"version", "--short"},
		InstallURL:  "https://helm.sh/docs/intro/install/"},
	{Name: "kubectl", Minimum: Version{1, 32, 0},
		VersionArgs: []string{"version", "--client=true", "-o", "json"},
		InstallURL:  "https://kubernetes.io/docs/tasks/tools/"},
}

type ToolStatus struct {
	Name       string
	Version    Version
	Minimum    Version
	InstallURL string
}

type PreflightReport struct {
	Tools     []ToolStatus
	CPUs      int
	MemoryGiB float64
}

func DefaultHostRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// CheckPreflight checks tool presence and versions plus macOS Docker resources.
// It runs only read-only version/info commands.
func CheckPreflight(ctx context.Context, run HostRunner, goos string) (PreflightReport, error) {
	var report PreflightReport
	for _, requirement := range StandardToolchain {
		commandCtx, cancel := context.WithTimeout(ctx, preflightCommandTimeout)
		output, err := run(commandCtx, requirement.Name, requirement.VersionArgs...)
		cancel()
		if err != nil {
			return report, fmt.Errorf(
				"%s unavailable or not running: %w: %s; install: %s",
				requirement.Name, err, strings.TrimSpace(string(output)),
				requirement.InstallURL)
		}
		version, err := parseVersion(string(output))
		if err != nil {
			return report, fmt.Errorf("%s version: %w", requirement.Name, err)
		}
		if version.less(requirement.Minimum) {
			return report, fmt.Errorf(
				"%s %s is below required %s; install: %s",
				requirement.Name, version, requirement.Minimum, requirement.InstallURL)
		}
		report.Tools = append(report.Tools, ToolStatus{
			Name: requirement.Name, Version: version, Minimum: requirement.Minimum,
			InstallURL: requirement.InstallURL,
		})
	}
	commandCtx, cancel := context.WithTimeout(ctx, preflightCommandTimeout)
	output, err := run(commandCtx, "docker", "info", "--format", "{{.NCPU}} {{.MemTotal}}")
	cancel()
	if err != nil {
		return report, fmt.Errorf("the Docker daemon is unavailable: %w: %s",
			err, strings.TrimSpace(string(output)))
	}
	if goos == "darwin" {
		fields := strings.Fields(string(output))
		if len(fields) != 2 {
			return report, fmt.Errorf("parse Docker Desktop resources %q", strings.TrimSpace(string(output)))
		}
		cpus, cpuErr := strconv.Atoi(fields[0])
		memory, memoryErr := strconv.ParseInt(fields[1], 10, 64)
		if cpuErr != nil || memoryErr != nil {
			return report, fmt.Errorf("parse Docker Desktop resources %q", strings.TrimSpace(string(output)))
		}
		report.CPUs = cpus
		report.MemoryGiB = float64(memory) / (1 << 30)
		if cpus < MinimumDockerCPUs || report.MemoryGiB < MinimumDockerMemoryGiB {
			return report, fmt.Errorf(
				"the Docker Desktop resources %.1f GiB/%d CPUs are below the required %d GiB/%d CPUs",
				report.MemoryGiB, cpus, MinimumDockerMemoryGiB, MinimumDockerCPUs)
		}
	}
	return report, nil
}

func Doctor() error {
	report, err := CheckPreflight(context.Background(), DefaultHostRun, runtime.GOOS)
	if err != nil {
		return err
	}
	for _, tool := range report.Tools {
		fmt.Printf("doctor: %s %s (minimum %s) OK\n", tool.Name, tool.Version, tool.Minimum)
	}
	if runtime.GOOS == "darwin" {
		fmt.Printf("doctor: Docker Desktop %.1f GiB/%d CPUs OK\n", report.MemoryGiB, report.CPUs)
	}
	fmt.Println("doctor PASS - toolchain versions and host resources satisfy ENG01")
	return nil
}

var versionPattern = regexp.MustCompile(`(?i)\bv?(\d+)\.(\d+)(?:\.(\d+))?`)

func parseVersion(raw string) (Version, error) {
	match := versionPattern.FindStringSubmatch(raw)
	if len(match) == 0 {
		return Version{}, fmt.Errorf("cannot parse semantic version from %q", strings.TrimSpace(raw))
	}
	values := [3]int{}
	for index := range values {
		if match[index+1] == "" {
			continue
		}
		value, err := strconv.Atoi(match[index+1])
		if err != nil {
			return Version{}, err
		}
		values[index] = value
	}
	return Version{values[0], values[1], values[2]}, nil
}

func (v Version) less(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}
