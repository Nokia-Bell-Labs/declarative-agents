// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"runtime"
	"strings"
	"testing"
)

func TestCLIDonorImageIsDigestPinned(t *testing.T) {
	name, digest, ok := strings.Cut(CLIDonorImage, "@")
	if !ok || !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("CLIDonorImage %q is not pinned by a sha256 digest", CLIDonorImage)
	}
	if !strings.HasSuffix(name, ":"+strings.TrimPrefix(CLIDonorRuntimeImage, "kindrig/cli-donor:")) {
		t.Fatalf("donor %q and rig-local tag %q name different versions", name, CLIDonorRuntimeImage)
	}
}

func TestEnsureCLIDonorImageReusesLocalDigest(t *testing.T) {
	cluster := &fakeCluster{}
	if err := EnsureCLIDonorImage(cluster.run, "da-platform"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker image inspect --format {{.Id}} " + CLIDonorImage,
		"docker tag " + CLIDonorImage + " " + CLIDonorRuntimeImage,
		"kind load docker-image " + CLIDonorRuntimeImage + " --name da-platform",
	}
	if strings.Join(cluster.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(cluster.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestEnsureCLIDonorImagePullsAbsentDigest(t *testing.T) {
	cluster := &fakeCluster{fail: map[string]string{"docker image inspect": "No such image"}}
	if err := EnsureCLIDonorImage(cluster.run, "da-platform"); err != nil {
		t.Fatal(err)
	}
	if len(cluster.calls) < 2 ||
		cluster.calls[1] != "docker pull --platform linux/"+runtime.GOARCH+" "+CLIDonorImage {
		t.Fatalf("absent donor was not pulled: %v", cluster.calls)
	}
}

func TestEnsureCLIDonorImageNamesTheFailedStep(t *testing.T) {
	cluster := &fakeCluster{fail: map[string]string{"kind load": "no nodes found"}}
	err := EnsureCLIDonorImage(cluster.run, "da-platform")
	if err == nil || !strings.Contains(err.Error(), "ensure CLI donor") ||
		!strings.Contains(err.Error(), "no nodes found") {
		t.Fatalf("error = %v, want the failed load named", err)
	}
	if err := EnsureCLIDonorImage(cluster.run, " "); err == nil {
		t.Fatal("blank cluster accepted")
	}
}
