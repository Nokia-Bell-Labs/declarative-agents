// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"strings"
	"testing"
)

type pinnedImageDaemon struct {
	calls   []string
	images  map[string]bool
	failAt  string
	failOut string
}

func newPinnedImageDaemon(present ...string) *pinnedImageDaemon {
	images := map[string]bool{}
	for _, image := range present {
		images[image] = true
	}
	return &pinnedImageDaemon{images: images}
}

func (d *pinnedImageDaemon) run(name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	d.calls = append(d.calls, call)
	if d.failAt != "" && strings.Contains(call, d.failAt) {
		return []byte(d.failOut), errors.New("command failed")
	}
	if name == "docker" && len(args) >= 3 && args[0] == "image" && args[1] == "inspect" {
		image := args[len(args)-1]
		if d.images[image] {
			return []byte("sha256:present"), nil
		}
		return []byte("No such image"), errors.New("absent")
	}
	if name == "docker" && len(args) >= 3 && args[0] == "pull" {
		d.images[args[len(args)-1]] = true
		return nil, nil
	}
	if name == "docker" && len(args) >= 3 && args[0] == "tag" {
		d.images[args[2]] = true
		return nil, nil
	}
	if name == "docker" && len(args) >= 3 && args[0] == "image" && args[1] == "rm" {
		delete(d.images, args[2])
		return nil, nil
	}
	return nil, nil
}

func TestPinnedImageReferencesDistinguishSourceHostAndNode(t *testing.T) {
	refs, err := PinnedImageReferences(
		"docker.io/library/traefik:v3.7.10@sha256:"+strings.Repeat("a", 64),
		"kindrig/traefik:v3.7.10")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(refs.Source, "@sha256:") {
		t.Fatalf("source = %q, want the digest-pinned cache name", refs.Source)
	}
	if refs.HostImport != "kindrig/traefik:v3.7.10" {
		t.Fatalf("host import = %q", refs.HostImport)
	}
	if refs.Node != "docker.io/kindrig/traefik:v3.7.10" {
		t.Fatalf("node = %q, want containerd's docker.io prefix", refs.Node)
	}
	if _, err := PinnedImageReferences("", "kindrig/traefik:v3.7.10"); err == nil {
		t.Fatal("empty source accepted")
	}
	if _, err := PinnedImageReferences(refs.Source, refs.Source); err == nil {
		t.Fatal("digest host import tag accepted")
	}
}

func TestNormalizeNodeImageReferenceMatchesContainerdAndKubernetes(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"traefik:v3.7.10", "docker.io/library/traefik:v3.7.10"},
		{"library/traefik:v3.7.10", "docker.io/library/traefik:v3.7.10"},
		{"docker.io/library/traefik:v3.7.10", "docker.io/library/traefik:v3.7.10"},
		{"kindrig/traefik:v3.7.10", "docker.io/kindrig/traefik:v3.7.10"},
		{"docker.io/kindrig/cli-donor:1.31.4", "docker.io/kindrig/cli-donor:1.31.4"},
		{"registry.k8s.io/metrics-server/metrics-server:v0.9.0",
			"registry.k8s.io/metrics-server/metrics-server:v0.9.0"},
		{"localhost/declarative-agents/runtime/agent-core:git-0123456789ab-linux-arm64",
			"localhost/declarative-agents/runtime/agent-core:git-0123456789ab-linux-arm64"},
		{"index.docker.io/library/busybox:1.36", "docker.io/library/busybox:1.36"},
		{"docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("b", 64),
			"docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("b", 64)},
	}
	for _, test := range tests {
		if got := NormalizeNodeImageReference(test.in); got != test.want {
			t.Errorf("NormalizeNodeImageReference(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestImportPinnedImageSkipsPullAndRemovesCreatedHostTag(t *testing.T) {
	source := "docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("a", 64)
	host := "kindrig/traefik:v3.7.10"
	daemon := newPinnedImageDaemon(source)
	if err := importPinnedImage(daemon.run, "da-platform", "traefik", source, host); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker image inspect --format {{.Id}} " + source,
		"docker image inspect --format {{.Id}} " + host,
		"docker tag " + source + " " + host,
		"node-import " + host + " da-platform-control-plane " + HostPlatform(),
		"docker image rm " + host,
	}
	if len(daemon.calls) != len(want) {
		t.Fatalf("calls = %v, want %d", daemon.calls, len(want))
	}
	for i, expected := range want {
		if !strings.Contains(daemon.calls[i], expected) {
			t.Errorf("call[%d] = %q, want %q", i, daemon.calls[i], expected)
		}
	}
	if !strings.Contains(daemon.calls[3], "import --platform=") ||
		!strings.Contains(daemon.calls[3], HostPlatform()) {
		t.Fatalf("import is not host-platform scoped: %s", daemon.calls[3])
	}
	if !daemon.images[source] {
		t.Fatal("source digest was removed")
	}
	if daemon.images[host] {
		t.Fatal("created host import tag remained after success")
	}
}

func TestImportPinnedImageRetainsCanonicalUpstreamTag(t *testing.T) {
	source := "docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("a", 64)
	host := "docker.io/library/traefik:v3.7.10"
	daemon := newPinnedImageDaemon(source)
	if err := importPinnedImage(daemon.run, "da-platform", "traefik", source, host); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(daemon.calls, "\n")
	if strings.Contains(joined, "docker image rm ") {
		t.Fatalf("canonical tag was untagged: %v", daemon.calls)
	}
	if !daemon.images[source] || !daemon.images[host] {
		t.Fatalf("canonical pin missing after import: %v", daemon.images)
	}
}

func TestImportPinnedImagePullsAbsentSourceThenUntags(t *testing.T) {
	source := "docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("c", 64)
	host := "kindrig/traefik:v3.7.10"
	daemon := newPinnedImageDaemon()
	if err := importPinnedImage(daemon.run, "da-platform", "traefik", source, host); err != nil {
		t.Fatal(err)
	}
	if len(daemon.calls) < 3 ||
		daemon.calls[1] != "docker pull --platform "+HostPlatform()+" "+source {
		t.Fatalf("absent source was not pulled: %v", daemon.calls)
	}
	if !strings.Contains(strings.Join(daemon.calls, "\n"), "docker image rm "+host) {
		t.Fatalf("created host tag was not removed: %v", daemon.calls)
	}
	if !daemon.images[source] || daemon.images[host] {
		t.Fatalf("images after import: %v", daemon.images)
	}
}

func TestImportPinnedImagePreservesPreExistingHostTag(t *testing.T) {
	source := "docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("d", 64)
	host := "kindrig/traefik:v3.7.10"
	daemon := newPinnedImageDaemon(source, host)
	if err := importPinnedImage(daemon.run, "da-platform", "traefik", source, host); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(daemon.calls, "\n")
	if strings.Contains(joined, "docker tag ") || strings.Contains(joined, "docker image rm ") {
		t.Fatalf("pre-existing host tag was rewritten: %v", daemon.calls)
	}
	if !strings.Contains(joined, "node-import "+host) {
		t.Fatalf("pre-existing tag was not imported: %v", daemon.calls)
	}
	if !daemon.images[source] || !daemon.images[host] {
		t.Fatal("pre-existing host tag or source digest was removed")
	}
}

func TestImportPinnedImageRollsBackCreatedTagWhenImportFails(t *testing.T) {
	source := "docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("e", 64)
	host := "kindrig/traefik:v3.7.10"
	daemon := newPinnedImageDaemon(source)
	daemon.failAt = "node-import"
	daemon.failOut = "node not found"
	err := importPinnedImage(daemon.run, "da-platform", "traefik", source, host)
	if err == nil || !strings.Contains(err.Error(), "node not found") {
		t.Fatalf("error = %v, want the failed import", err)
	}
	if daemon.images[host] {
		t.Fatal("failed import left the created host tag")
	}
	if !daemon.images[source] {
		t.Fatal("failed import removed the source digest")
	}
}

func TestImportPinnedImageDoesNotRemovePreExistingTagOnFailure(t *testing.T) {
	source := "docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("f", 64)
	host := "kindrig/traefik:v3.7.10"
	daemon := newPinnedImageDaemon(source, host)
	daemon.failAt = "node-import"
	daemon.failOut = "import failed"
	if err := importPinnedImage(daemon.run, "da-platform", "traefik", source, host); err == nil {
		t.Fatal("failed import succeeded")
	}
	if !daemon.images[host] || !daemon.images[source] {
		t.Fatalf("failure removed a preserved image: %v", daemon.images)
	}
}

func inspectFailsWithoutDigest(name string, args []string) bool {
	if name != "docker" || len(args) < 3 || args[1] != "inspect" {
		return false
	}
	return !strings.Contains(args[len(args)-1], "@sha256:")
}

func TestPinnedImageStepsMatchImportOrder(t *testing.T) {
	source := "docker.io/fsouza/fake-gcs-server:1.56.1@sha256:" + strings.Repeat("1", 64)
	host := "kindrig/fake-gcs-server:1.56.1"
	daemon := newPinnedImageDaemon(source)
	steps := pinnedImageSteps(daemon.run, "da-platform", source, host)
	if len(steps) != 3 || steps[0].phase != "image-tag" ||
		steps[1].phase != "image-load" || steps[2].phase != "image-untag" {
		t.Fatalf("steps = %#v, want tag, load, untag", steps)
	}
	if got := strings.Join(steps[1].command, " "); !strings.Contains(got, HostPlatform()) {
		t.Fatalf("load command missing host platform: %s", got)
	}
}

func TestPinnedImageStepsRetainCanonicalUpstreamTag(t *testing.T) {
	source := "docker.io/library/traefik:v3.7.10@sha256:" + strings.Repeat("1", 64)
	host := "docker.io/library/traefik:v3.7.10"
	daemon := newPinnedImageDaemon(source)
	steps := pinnedImageSteps(daemon.run, "da-platform", source, host)
	if len(steps) != 2 || steps[0].phase != "image-tag" || steps[1].phase != "image-load" {
		t.Fatalf("steps = %#v, want tag and load without untag", steps)
	}
}
