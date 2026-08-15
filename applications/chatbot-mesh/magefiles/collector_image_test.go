// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCollectorImplementationsRender(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	tests := []struct {
		implementation string
		want           []string
		notWant        []string
	}{
		{
			implementation: "agent",
			want: []string{
				"agents/collector/profile.yaml",
				"--otel-metric-otlp-endpoint",
			},
			// The declarative collector owns OTLP metric intake (GH-1207), so agent
			// mode deploys no contrib collector and no separate metrics gateway
			// (GH-1366).
			notWant: []string{
				"collector-metrics",
				"opentelemetry-collector-contrib",
			},
		},
		{
			implementation: "contrib",
			want:           []string{"--config=/etc/collector/config.yaml", "name: otlp-http"},
			notWant:        []string{"agents/collector/profile.yaml", "--otel-metric-otlp-endpoint"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.implementation, func(t *testing.T) {
			out, err := exec.Command("helm", "template", "test", chartDir,
				"--set", "collector.implementation="+tt.implementation).CombinedOutput()
			if err != nil {
				t.Fatalf("render collector implementation %s: %v\n%s", tt.implementation, err, out)
			}
			rendered := string(out)
			for _, want := range tt.want {
				if !strings.Contains(rendered, want) {
					t.Errorf("rendered %s collector missing %q", tt.implementation, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(rendered, notWant) {
					t.Errorf("rendered %s collector unexpectedly contains %q", tt.implementation, notWant)
				}
			}
		})
	}
}

// This is the check that would have caught GH-736. The chart pinned
// otel/opentelemetry-collector-contrib:0.116.0, whose arm64 image ships a
// dynamically linked binary needing /lib/ld-linux-aarch64.so.1 -- a loader a
// distroless image does not carry. The pod exited 255 with
// "exec /otelcol-contrib: no such file or directory" on every Apple Silicon
// host, so no agent span reached the trace backend and the smoke failed two
// hops away, at an empty spool.
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

// knownBadCollectorImage is the GH-736 pin: its arm64 binary is dynamically
// linked against a loader the distroless image does not ship.
const knownBadCollectorImage = "otel/opentelemetry-collector-contrib:0.116.0"

func pinnedCollectorImageSkipReason(lookPath func(string) (string, error), dockerInfo func() error) string {
	if _, err := lookPath("docker"); err != nil {
		return "docker not on PATH"
	}
	if err := dockerInfo(); err != nil {
		return "docker daemon is not running"
	}
	return ""
}

func dockerDaemonSkipReason(out []byte, err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(string(out) + " " + err.Error())
	if strings.Contains(text, "cannot connect to the docker daemon") ||
		strings.Contains(text, "is the docker daemon running") {
		return "docker daemon is not running"
	}
	return ""
}

func pinnedCollectorImageCheck(ref string, out []byte, err error) error {
	if err != nil {
		return fmt.Errorf("pinned collector image %s does not run here: %v\n%s\n"+
			"An exec error naming a missing interpreter means the image's binary for this "+
			"architecture is dynamically linked against a loader the image does not ship; "+
			"pin a release whose build is static (GH-736).", ref, err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), "otelcol") {
		return fmt.Errorf("collector %s --version printed %q, want an otelcol version banner",
			ref, strings.TrimSpace(string(out)))
	}
	return nil
}

func hostDockerInfo() error {
	return exec.Command("docker", "info").Run()
}

// TestPinnedCollectorImageRunsOnThisArchitecture executes the pinned collector
// on the host architecture. A chart that renders perfectly is still broken when
// its image cannot exec, and that failure only ever appears on the architecture
// that carries the bad build. Since GH-1366 the contrib image is opt-in
// (implementation: contrib), so this guards the opt-in path; it stays because a
// contrib install must still exec on Apple Silicon.
func TestPinnedCollectorImageRunsOnThisArchitecture(t *testing.T) {
	t.Parallel()
	if reason := pinnedCollectorImageSkipReason(exec.LookPath, hostDockerInfo); reason != "" {
		t.Skip(reason)
	}
	ref := collectorImageRef(t)
	out, err := exec.Command("docker", "run", "--rm", ref, "--version").CombinedOutput()
	if reason := dockerDaemonSkipReason(out, err); reason != "" {
		t.Skip(reason)
	}
	if checkErr := pinnedCollectorImageCheck(ref, out, err); checkErr != nil {
		t.Fatal(checkErr)
	}
}

func TestPinnedCollectorImageSkipReasonNamesDocker(t *testing.T) {
	t.Parallel()
	t.Run("not on PATH", func(t *testing.T) {
		infoCalled := false
		reason := pinnedCollectorImageSkipReason(
			func(string) (string, error) { return "", exec.ErrNotFound },
			func() error { infoCalled = true; return nil },
		)
		if infoCalled {
			t.Fatal("docker info must not run when docker is absent")
		}
		if !strings.Contains(strings.ToLower(reason), "docker") {
			t.Fatalf("skip reason = %q, want docker named", reason)
		}
	})
	t.Run("daemon not running", func(t *testing.T) {
		reason := pinnedCollectorImageSkipReason(
			func(string) (string, error) { return "/usr/bin/docker", nil },
			func() error { return errors.New("Cannot connect to the Docker daemon") },
		)
		if !strings.Contains(strings.ToLower(reason), "docker") {
			t.Fatalf("skip reason = %q, want docker named", reason)
		}
	})
}

func TestPinnedCollectorImageKnownBadReleaseFails(t *testing.T) {
	t.Parallel()
	out := []byte("exec /otelcol-contrib: no such file or directory")
	runErr := errors.New("exit status 255")
	if reason := dockerDaemonSkipReason(out, runErr); reason != "" {
		t.Fatalf("skip = %q, want fail for known-bad pin %s", reason, knownBadCollectorImage)
	}
	err := pinnedCollectorImageCheck(knownBadCollectorImage, out, runErr)
	if err == nil {
		t.Fatal("known-bad release must fail")
	}
	if !strings.Contains(err.Error(), knownBadCollectorImage) || !strings.Contains(err.Error(), "GH-736") {
		t.Fatalf("error = %v, want the known-bad pin and GH-736", err)
	}
}

func TestPinnedCollectorImageDockerRunDaemonErrorSkips(t *testing.T) {
	t.Parallel()
	out := []byte("Cannot connect to the Docker daemon at unix:///Users/x/.docker/run/docker.sock. Is the docker daemon running?")
	reason := dockerDaemonSkipReason(out, errors.New("exit status 1"))
	if !strings.Contains(strings.ToLower(reason), "docker") {
		t.Fatalf("skip reason = %q, want docker named", reason)
	}
}
