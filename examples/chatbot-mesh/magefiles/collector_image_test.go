// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// This is the check that would have caught GH-736. The chart pinned
// otel/opentelemetry-collector-contrib:0.116.0, whose arm64 image ships a
// dynamically linked binary needing /lib/ld-linux-aarch64.so.1 -- a loader a
// distroless image does not carry. The pod exited 255 with
// "exec /otelcol-contrib: no such file or directory" on every Apple Silicon
// host, so no agent span reached the trace backend and the smoke failed two
// hops away, at an empty Jaeger service list.
//
// Nothing in the chart or its rendered output was wrong, which is why a render
// test could not find it: the image reference was valid and the manifest listed
// an arm64 variant. Only running the image shows it.

// collectorImageRef reads the pinned collector image from the chart values, so
// the guard follows a bump rather than restating one.
func collectorImageRef(t *testing.T) string {
	t.Helper()
	chartDir := findChartDir(t)
	data, err := exec.Command("cat", chartDir+"/values.yaml").Output()
	if err != nil {
		t.Fatalf("read values.yaml: %v", err)
	}
	var values struct {
		Collector struct {
			Image struct {
				Repository string `yaml:"repository"`
				Tag        string `yaml:"tag"`
			} `yaml:"image"`
		} `yaml:"collector"`
	}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatalf("parse values.yaml: %v", err)
	}
	ref := values.Collector.Image.Repository + ":" + values.Collector.Image.Tag
	if ref == ":" {
		t.Fatal("no collector image in values.yaml; the guard would pass vacuously")
	}
	return ref
}

// TestPinnedCollectorImageRunsOnThisArchitecture executes the pinned collector
// on the host architecture. A chart that renders perfectly is still broken when
// its image cannot exec, and that failure only ever appears on the architecture
// that carries the bad build.
func TestPinnedCollectorImageRunsOnThisArchitecture(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}
	ref := collectorImageRef(t)

	out, err := exec.Command("docker", "run", "--rm", ref, "--version").CombinedOutput()

	if err != nil {
		t.Fatalf("pinned collector image %s does not run here: %v\n%s\n"+
			"An exec error naming a missing interpreter means the image's binary for this "+
			"architecture is dynamically linked against a loader the image does not ship; "+
			"pin a release whose build is static (GH-736).", ref, err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), "otelcol") {
		t.Errorf("collector %s --version printed %q, want an otelcol version banner",
			ref, strings.TrimSpace(string(out)))
	}
}
