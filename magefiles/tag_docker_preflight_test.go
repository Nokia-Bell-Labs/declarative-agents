// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseDockerStatsMemoryReadsUsedColumn(t *testing.T) {
	got, err := parseDockerStatsMemory("da-agentic-wiki-mesh-demo-control-plane|2.019GiB / 7.75GiB\n" +
		"da-coding-agent-helm-control-plane|869.8MiB / 7.75GiB\nidle|512KiB / 7.75GiB\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []dockerContainerMemory{
		{name: "da-agentic-wiki-mesh-demo-control-plane", bytes: scaledBytes(2.019, 1<<30)},
		{name: "da-coding-agent-helm-control-plane", bytes: scaledBytes(869.8, 1<<20)},
		{name: "idle", bytes: 512 << 10},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("containers = %+v, want %+v", got, want)
	}
	if got, err := parseDockerStatsMemory("\n"); err != nil || len(got) != 0 {
		t.Fatalf("empty stats = %v, %v; want none", got, err)
	}
}

func TestParseDockerStatsMemoryRejectsUnknownUnits(t *testing.T) {
	if _, err := parseDockerStatsMemory("c|12parsecs / 7.75GiB"); err == nil {
		t.Fatal("unknown unit parsed")
	}
	if _, err := parseDockerStatsMemory("no memory column"); err == nil {
		t.Fatal("line without a memory column parsed")
	}
}

// The attempt-4 VM: 7.75 GiB with a foreign demo cluster and two idle smoke
// clusters leaves about 4 GiB, under the floor, and the error names every holder
// largest first.
func TestReleaseDockerHeadroomRefusesAStarvedVM(t *testing.T) {
	probe := func() (int64, []dockerContainerMemory, error) {
		return 8321232896, []dockerContainerMemory{
			{name: "da-agent-architecture-helm-control-plane", bytes: 849 << 20},
			{name: "da-agentic-wiki-mesh-demo-control-plane", bytes: 1967 << 20},
			{name: "da-coding-agent-helm-control-plane", bytes: 905 << 20},
		}, nil
	}
	err := checkReleaseDockerHeadroom(probe, releaseDockerFreeFloor)
	if err == nil {
		t.Fatal("starved VM passed the preflight")
	}
	msg := err.Error()
	order := []string{"da-agentic-wiki-mesh-demo", "da-coding-agent-helm", "da-agent-architecture-helm"}
	last := -1
	for _, name := range order {
		at := strings.Index(msg, name)
		if at <= last {
			t.Fatalf("holders not listed largest first in %q", msg)
		}
		last = at
	}
	if !strings.Contains(msg, "need 5.00 GiB") {
		t.Fatalf("error does not state the floor: %q", msg)
	}
}

func TestReleaseDockerHeadroomPassesWithRoomAndSurfacesProbeErrors(t *testing.T) {
	roomy := func() (int64, []dockerContainerMemory, error) {
		return 8321232896, []dockerContainerMemory{{name: "demo", bytes: 2 << 30}}, nil
	}
	if err := checkReleaseDockerHeadroom(roomy, releaseDockerFreeFloor); err != nil {
		t.Fatalf("roomy VM refused: %v", err)
	}
	down := func() (int64, []dockerContainerMemory, error) { return 0, nil, errors.New("daemon down") }
	if err := checkReleaseDockerHeadroom(down, releaseDockerFreeFloor); err == nil ||
		!strings.Contains(err.Error(), "daemon down") {
		t.Fatalf("probe error = %v, want daemon down", err)
	}
}

func scaledBytes(value, scale float64) int64 { return int64(value * scale) }
