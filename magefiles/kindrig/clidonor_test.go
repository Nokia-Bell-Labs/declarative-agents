// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestCLIDonorImageIsDigestPinned(t *testing.T) {
	name, digest, ok := strings.Cut(CLIDonorImage, "@")
	if !ok || !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("CLIDonorImage %q is not pinned by a sha256 digest", CLIDonorImage)
	}
	if tagWithoutDigest(CLIDonorImage) != CLIDonorRuntimeImage {
		t.Fatalf("donor %q and rig-local tag %q name different versions", name, CLIDonorRuntimeImage)
	}
}

func TestEnsureCLIDonorImageReusesLocalDigest(t *testing.T) {
	cluster := &fakeCluster{}
	origRun := cluster.run
	clusterRun := func(name string, args ...string) ([]byte, error) {
		if inspectFailsWithoutDigest(name, args) {
			cluster.calls = append(cluster.calls, name+" "+strings.Join(args, " "))
			return []byte("No such image"), errors.New("absent")
		}
		return origRun(name, args...)
	}
	if err := EnsureCLIDonorImage(clusterRun, "da-platform"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker image inspect --format {{.Id}} " + CLIDonorImage,
		"docker image inspect --format {{.Id}} " + CLIDonorRuntimeImage,
		"docker tag " + CLIDonorImage + " " + CLIDonorRuntimeImage,
		"node-import " + CLIDonorRuntimeImage + " da-platform-control-plane linux/" + runtime.GOARCH,
	}
	if len(cluster.calls) != len(want) {
		t.Fatalf("calls = %v, want %d", cluster.calls, len(want))
	}
	for i, call := range cluster.calls {
		if !strings.Contains(call, want[i]) {
			t.Fatalf("call[%d] = %q, want %q", i, call, want[i])
		}
	}
	if !strings.Contains(cluster.calls[3], "ctr --namespace=k8s.io images import --platform=") {
		t.Fatalf("donor load is not a platform-scoped node import: %s", cluster.calls[3])
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
	if strings.Contains(strings.Join(cluster.calls, "\n"), "docker image rm "+CLIDonorRuntimeImage) {
		t.Fatalf("canonical upstream tag was untagged: %v", cluster.calls)
	}
}

func TestEnsureCLIDonorImageNamesTheFailedStep(t *testing.T) {
	cluster := &fakeCluster{fail: map[string]string{"sh -c": "no nodes found"}}
	err := EnsureCLIDonorImage(cluster.run, "da-platform")
	if err == nil || !strings.Contains(err.Error(), "ensure CLI donor") ||
		!strings.Contains(err.Error(), "no nodes found") {
		t.Fatalf("error = %v, want the failed load named", err)
	}
	if err := EnsureCLIDonorImage(cluster.run, " "); err == nil {
		t.Fatal("blank cluster accepted")
	}
}

// donorContainer fakes kubectl exec into an applier container.
type donorContainer struct {
	helm     string
	writable bool
	writeErr string
}

func (c donorContainer) exec(args ...string) (string, error) {
	switch args[0] {
	case "helm":
		return c.helm, nil
	case "kubectl":
		return "Client Version: v1.31.4", nil
	case "sh":
		if c.writable {
			return "", nil
		}
		return c.writeErr, errors.New("command terminated with exit code 1")
	}
	return "", errors.New("unexpected command")
}

func TestVerifyCLIDonor(t *testing.T) {
	readOnly := "touch: /opt/tools/cli-donor-write-probe: Read-only file system"
	tests := []struct {
		name      string
		container donorContainer
		want      string
	}{
		{"pinned and read-only", donorContainer{helm: CLIDonorHelmVersion, writeErr: readOnly}, ""},
		{"unpinned helm", donorContainer{helm: "v3.15.4", writeErr: readOnly}, "pinned CLI donor carries"},
		{"writable tools", donorContainer{helm: CLIDonorHelmVersion, writable: true}, "can write /opt/tools"},
		{"write fails for another reason", donorContainer{helm: CLIDonorHelmVersion, writeErr: "permission denied"}, "not on a read-only mount"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version, err := VerifyCLIDonor("3", test.container.exec)
			if test.want == "" {
				if err != nil || version != CLIDonorHelmVersion {
					t.Fatalf("version=%q err=%v", version, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	if _, err := VerifyCLIDonor("4", donorContainer{helm: CLIDonorHelmVersion, writeErr: readOnly}.exec); err == nil ||
		!strings.Contains(err.Error(), "written for helm 4") {
		t.Fatalf("major mismatch error = %v", err)
	}
}
