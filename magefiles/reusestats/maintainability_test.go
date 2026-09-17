// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package reusestats

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Two agents with different file counts, a shared unit, and one 301-line file
// pin every maintainability counter (GH-2111).
func TestCollectPinsMaintainabilityCounters(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"agents/small", "agents/large/tests", "agents/units"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeReuseFixture(t, root, "agents/small/profile.yaml", "name: small\nmachine: machine.yaml\n")
	writeReuseFixture(t, root, "agents/small/machine.yaml", "name: small\n")
	writeReuseFixture(t, root, "agents/large/profile.yaml", "name: large\nmachine: machine.yaml\n")
	writeReuseFixture(t, root, "agents/large/machine.yaml", "name: large\n")
	writeReuseFixture(t, root, "agents/large/declarations.yaml",
		"unit: large\nimports: [../units/shared.yaml, ../units/types.yaml]\ntools: []\n")
	writeReuseFixture(t, root, "agents/large/rest.yaml", "unit: large-rest\nimports: [../units/shared.yaml]\nrest: {}\n")
	writeReuseFixture(t, root, "agents/large/tests/case.yaml", "name: case\n")
	writeReuseFixture(t, root, "agents/units/shared.yaml", "unit: shared\ntools: []\n")
	writeReuseFixture(t, root, "agents/units/types.yaml", "unit: types\ntypes: []\n")
	writeReuseFixture(t, root, "agents/large/long.yaml", "notes:\n"+strings.Repeat("  - line\n", 300))

	got := mustCollect(t, root, "agents").Maintainability

	want := Maintainability{
		Files: 10, MedianLines: 2, P90Lines: 3, MaxLines: 301, FilesOver300: 1,
		LongestFiles: []FileSize{
			{Path: "agents/large/long.yaml", Lines: 301},
			{Path: "agents/large/declarations.yaml", Lines: 3},
			{Path: "agents/large/rest.yaml", Lines: 3},
			{Path: "agents/large/profile.yaml", Lines: 2},
			{Path: "agents/small/profile.yaml", Lines: 2},
			{Path: "agents/units/shared.yaml", Lines: 2},
			{Path: "agents/units/types.yaml", Lines: 2},
			{Path: "agents/large/machine.yaml", Lines: 1},
			{Path: "agents/large/tests/case.yaml", Lines: 1},
			{Path: "agents/small/machine.yaml", Lines: 1},
		},
		// small spans profile and machine; large spans five files, its tests/
		// subtree left out; agents/units belongs to no agent.
		Agents: 2, MedianFilesPerAgent: 2, MaxFilesPerAgent: 5,
		// declarations imports two units and rest one; shared has two
		// importers and types one.
		MedianImportFanOut: 1, MaxImportFanOut: 2,
		MedianUnitFanIn: 1, MaxUnitFanIn: 2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Maintainability = %#v, want %#v", got, want)
	}
}

// The repository fold concatenates samples, so its median is the median of all
// observations rather than a median of module medians.
func TestMergeFoldsSamplesExactly(t *testing.T) {
	merged := Merge(map[string]Samples{
		"b": {Files: []FileSize{{Path: "x.yaml", Lines: 100}}, FilesPerAgent: []int{9}},
		"a": {Files: []FileSize{{Path: "x.yaml", Lines: 1}, {Path: "y.yaml", Lines: 2}, {Path: "z.yaml", Lines: 3}},
			FilesPerAgent: []int{1, 2}},
	})
	summary := Summarize(merged)
	if summary.MedianLines != 2 || summary.MaxLines != 100 || summary.MedianFilesPerAgent != 2 {
		t.Fatalf("summary = %#v, want median 2 over all four files, max 100, agent median 2", summary)
	}
	if summary.LongestFiles[0].Path != "b/x.yaml" {
		t.Fatalf("longest = %#v, want the module-prefixed path", summary.LongestFiles[0])
	}
}

func TestMedianAndPercentileOnSmallSamples(t *testing.T) {
	for _, test := range []struct {
		values      []int
		median, p90 int
	}{
		{nil, 0, 0},
		{[]int{7}, 7, 7},
		{[]int{1, 2}, 1, 2},
		{[]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 5, 9},
		{[]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 6, 10},
	} {
		if got := median(test.values); got != test.median {
			t.Errorf("median(%v) = %d, want %d", test.values, got, test.median)
		}
		if got := percentile90(test.values); got != test.p90 {
			t.Errorf("percentile90(%v) = %d, want %d", test.values, got, test.p90)
		}
	}
}
