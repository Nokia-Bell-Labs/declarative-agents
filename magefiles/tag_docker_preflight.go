// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// releaseDockerFreeFloor is the Docker VM memory the release needs free before
// it boots da-platform: one kind control plane (about 1.3 GiB), the application
// pods of two concurrent docker lanes, and the in-cluster LLM tier. Release
// attempt 4 on GH-2120 started with about 4 GiB free and timed out chat
// inference and readiness probes; attempts with more headroom passed (GH-2134).
// That run paid three control planes; with one shared control plane (GH-2215)
// the same floor leaves more for application pods, and we keep the value until
// a release measures otherwise.
const releaseDockerFreeFloor = 5 << 30

// dockerContainerMemory is one running container's resident memory.
type dockerContainerMemory struct {
	name  string
	bytes int64
}

// dockerMemoryProbe reports the Docker VM's total memory and the memory held
// by each running container.
type dockerMemoryProbe func() (total int64, containers []dockerContainerMemory, err error)

// checkReleaseDockerHeadroom refuses the release when running containers leave
// less than floor bytes of the Docker VM free, naming the largest holders so
// the operator can decide which to stop.
func checkReleaseDockerHeadroom(probe dockerMemoryProbe, floor int64) error {
	total, containers, err := probe()
	if err != nil {
		return fmt.Errorf("release docker preflight: %w", err)
	}
	var held int64
	for _, c := range containers {
		held += c.bytes
	}
	free := total - held
	if free >= floor {
		fmt.Printf("release: docker headroom %s free of %s\n", formatGiB(free), formatGiB(total))
		return nil
	}
	return fmt.Errorf("release docker preflight: %s free of %s, need %s; running containers hold %s: %s",
		formatGiB(free), formatGiB(total), formatGiB(floor), formatGiB(held),
		describeDockerHolders(containers))
}

func describeDockerHolders(containers []dockerContainerMemory) string {
	sorted := append([]dockerContainerMemory(nil), containers...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].bytes > sorted[j].bytes })
	parts := make([]string, 0, len(sorted))
	for _, c := range sorted {
		parts = append(parts, fmt.Sprintf("%s %s", c.name, formatGiB(c.bytes)))
	}
	return strings.Join(parts, ", ")
}

func formatGiB(bytes int64) string {
	return fmt.Sprintf("%.2f GiB", float64(bytes)/float64(1<<30))
}

// probeDockerMemory reads the VM total from docker info and per-container usage
// from one docker stats sample.
func probeDockerMemory() (int64, []dockerContainerMemory, error) {
	info, err := exec.Command("docker", "info", "--format", "{{.MemTotal}}").Output()
	if err != nil {
		return 0, nil, fmt.Errorf("docker info: %w", err)
	}
	total, err := strconv.ParseInt(strings.TrimSpace(string(info)), 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("parse docker MemTotal %q: %w", strings.TrimSpace(string(info)), err)
	}
	stats, err := exec.Command("docker", "stats", "--no-stream", "--format", "{{.Name}}|{{.MemUsage}}").Output()
	if err != nil {
		return 0, nil, fmt.Errorf("docker stats: %w", err)
	}
	containers, err := parseDockerStatsMemory(string(stats))
	return total, containers, err
}

// parseDockerStatsMemory parses "name|used / limit" lines from docker stats.
func parseDockerStatsMemory(output string) ([]dockerContainerMemory, error) {
	var containers []dockerContainerMemory
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, usage, ok := strings.Cut(line, "|")
		used, _, _ := strings.Cut(usage, "/")
		if !ok {
			return nil, fmt.Errorf("docker stats line %q has no memory column", line)
		}
		bytes, err := parseDockerSize(strings.TrimSpace(used))
		if err != nil {
			return nil, fmt.Errorf("docker stats line %q: %w", line, err)
		}
		containers = append(containers, dockerContainerMemory{name: strings.TrimSpace(name), bytes: bytes})
	}
	return containers, nil
}

var dockerSizeUnits = []struct {
	suffix string
	scale  float64
}{
	{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40},
	{"kB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"B", 1},
}

// parseDockerSize converts docker's human sizes ("2.019GiB", "858.3MiB") to bytes.
func parseDockerSize(size string) (int64, error) {
	for _, unit := range dockerSizeUnits {
		number, found := strings.CutSuffix(size, unit.suffix)
		if !found {
			continue
		}
		value, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return 0, fmt.Errorf("parse size %q: %w", size, err)
		}
		return int64(value * unit.scale), nil
	}
	return 0, fmt.Errorf("parse size %q: unknown unit", size)
}
