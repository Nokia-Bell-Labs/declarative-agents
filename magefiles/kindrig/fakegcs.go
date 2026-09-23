// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// fake-gcs-server backs the objectstore family's gs:// driver on a kind
// cluster (srd059 R2.2): the local half of the one-local-one-cloud doctrine,
// a supporting service in the srd002 managed_service sense, installed once by
// the rig rather than triplicated through application charts. The words bind
// it through their explicit endpoint configuration field; no environment
// variable is involved on either side.
const (
	fakeGCSImageRepository = "docker.io/fsouza/fake-gcs-server"
	fakeGCSImageVersion    = "1.56.1"
	// fakeGCSImageIndexDigest is the multi-platform index the host objectstore
	// tests docker-run (agent-core/internal/tools/objectstore/fakegcs_test.go).
	// Kind imports the host platform's child manifest from fakeGCSImageDigests.
	fakeGCSImageIndexDigest = "sha256:797ce226d62f947c009dc40246b30cfb456b8473d8241407f9d6f2c04e4d69ef"
	fakeGCSImagePlaceholder = "KINDRIG_FAKEGCS_IMAGE"
	fakeGCSNamespace        = "fake-gcs"
	fakeGCSDeployment       = "fake-gcs"
	// FakeGCSEndpoint is the JSON API base the objectstore words declare as
	// their endpoint in kind values overlays. The service DNS name is stable
	// across scenarios, so the overlay value never changes per run.
	FakeGCSEndpoint = "http://fake-gcs.fake-gcs.svc:4443/storage/v1/"
)

var fakeGCSImageDigests = map[string]string{
	"amd64": "sha256:af5fcc12a29db42953c9659dc44b478c164b78f6303390cf25fc93037d78522b",
	"arm64": "sha256:d018b5cd630d9a213df9c8608ebb81c9f1087cecdefe1eacc79ff79a24bdc43a",
}

//go:embed fake-gcs-kind.yaml
var fakeGCSKindManifest string

// InstallFakeGCS installs the pinned fake-gcs-server into its own namespace
// on a kind cluster and returns a cleanup that removes only what this call
// installed. A healthy pre-existing instance is reused; an unhealthy one is
// refused rather than overwritten because its ownership is unknown. The
// sequence follows InstallMetricsServer: pull by per-architecture digest,
// import under the fully qualified upstream tag, apply the embedded
// manifest, and observe readiness with rollout status (ENG01: no sleep loops).
func InstallFakeGCS(run CommandRunner, cluster string) (func() error, error) {
	if strings.TrimSpace(cluster) == "" {
		return nil, fmt.Errorf("install fake-gcs-server: kind cluster name is required")
	}
	status, err := fakeGCSStatus(run, cluster)
	if err == nil && status == "True" {
		return func() error { return nil }, nil
	}
	if err == nil {
		return nil, fmt.Errorf("fake-gcs deployment already exists but is not healthy: status %q", status)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "notfound") {
		return nil, fmt.Errorf("read existing fake-gcs deployment: %w", err)
	}
	sourceImage, err := fakeGCSImage(runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	runtimeImage := fakeGCSRuntimeImage()
	manifest, err := SubstitutePinnedImage(fakeGCSKindManifest, fakeGCSImagePlaceholder, runtimeImage)
	if err != nil {
		return nil, err
	}
	path, removeFile, err := writeFakeGCSManifest(manifest)
	if err != nil {
		return nil, err
	}
	if err := importPinnedImage(run, cluster, "fake-gcs-server", sourceImage, runtimeImage); err != nil {
		_ = deleteFakeGCSManifest(run, cluster, path)
		removeFile()
		return nil, err
	}
	steps := []installStep{
		{"manifest-apply", []string{"kubectl", "apply", "-f", path}},
		{"rollout", []string{"kubectl", "rollout", "status",
			"deployment/" + fakeGCSDeployment,
			"--namespace", fakeGCSNamespace, "--timeout=180s"}},
	}
	if err := runInstallSteps(run, cluster, "fake-gcs-server", steps); err != nil {
		_ = deleteFakeGCSManifest(run, cluster, path)
		removeFile()
		return nil, err
	}
	return func() error {
		defer removeFile()
		return deleteFakeGCSManifest(run, cluster, path)
	}, nil
}

func fakeGCSStatus(run CommandRunner, cluster string) (string, error) {
	command := inCluster(cluster, []string{"kubectl", "get", "deployment", fakeGCSDeployment,
		"--namespace", fakeGCSNamespace,
		"-o", `jsonpath={.status.conditions[?(@.type=="Available")].status}`})
	out, err := run(command[0], command[1:]...)
	status := strings.TrimSpace(string(out))
	if err != nil {
		return status, fmt.Errorf("%w: %s", err, status)
	}
	if status == "" {
		// The deployment exists but has not reported the condition; treat
		// it as unhealthy rather than absent, so ownership stays with
		// whoever created it.
		return status, nil
	}
	return status, nil
}

func fakeGCSImage(arch string) (string, error) {
	digest, ok := fakeGCSImageDigests[arch]
	if !ok {
		return "", fmt.Errorf("install fake-gcs-server: %s has no pinned digest for linux/%s",
			fakeGCSImageVersion, arch)
	}
	return fakeGCSImageRepository + ":" + fakeGCSImageVersion + "@" + digest, nil
}

func fakeGCSRuntimeImage() string {
	return NormalizeNodeImageReference(fakeGCSImageRepository + ":" + fakeGCSImageVersion)
}

func writeFakeGCSManifest(manifest string) (string, func(), error) {
	file, err := os.CreateTemp("", "kindrig-fake-gcs-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("create fake-gcs manifest: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.WriteString(manifest); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("write fake-gcs manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close fake-gcs manifest: %w", err)
	}
	return path, cleanup, nil
}

func deleteFakeGCSManifest(run CommandRunner, cluster, path string) error {
	command := inCluster(cluster, []string{"kubectl", "delete", "-f", path, "--ignore-not-found"})
	out, err := run(command[0], command[1:]...)
	if err != nil {
		return fmt.Errorf("delete fake-gcs manifest: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
