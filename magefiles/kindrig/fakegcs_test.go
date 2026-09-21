// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestInstallFakeGCSLoadsPinnedImageAndWaitsForRollout(t *testing.T) {
	var calls []string
	var applied string
	run := func(name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		calls = append(calls, command)
		if strings.HasPrefix(command, "kubectl get deployment") {
			return []byte(`Error from server (NotFound): deployments.apps "fake-gcs" not found`), errors.New("NotFound")
		}
		if name == "kubectl" && len(args) == 3 && args[0] == "apply" {
			data, err := os.ReadFile(args[2])
			if err != nil {
				return nil, err
			}
			applied = string(data)
		}
		return nil, nil
	}
	cleanup, err := InstallFakeGCS(run, "da-example")
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	source, err := fakeGCSImage(runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	runtimeImage := fakeGCSRuntimeRepository + ":" + fakeGCSImageVersion
	want := []string{
		"kubectl get deployment " + fakeGCSDeployment,
		"docker image inspect --format {{.Id}} " + source,
		"docker tag " + source + " " + runtimeImage,
		"node-import " + runtimeImage + " da-example-control-plane linux/" + runtime.GOARCH,
		"kubectl apply -f ",
		"kubectl rollout status deployment/" + fakeGCSDeployment +
			" --namespace " + fakeGCSNamespace + " --timeout=180s",
		"kubectl delete -f ",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %d", calls, len(want))
	}
	for index, expected := range want {
		if !strings.Contains(calls[index], expected) {
			t.Errorf("call[%d] = %q, want %q", index, calls[index], expected)
		}
	}
	for _, expected := range []string{
		"image: " + runtimeImage,
		"imagePullPolicy: Never",
		"-public-host",
		"fake-gcs.fake-gcs.svc",
		"-backend",
		"readinessProbe",
		"app.kubernetes.io/managed-by: kindrig",
	} {
		if !strings.Contains(applied, expected) {
			t.Errorf("applied manifest missing %q", expected)
		}
	}
	for _, forbidden := range []string{"imagePullPolicy: Always", "latest", "STORAGE_EMULATOR_HOST"} {
		if strings.Contains(applied, forbidden) {
			t.Errorf("applied manifest contains %q", forbidden)
		}
	}
}

// A healthy existing instance is reused without any install step; an
// unhealthy one is refused, because its ownership is unknown.
func TestInstallFakeGCSReusesHealthyAndRefusesUnhealthy(t *testing.T) {
	healthy := func(name string, args ...string) ([]byte, error) {
		return []byte("True"), nil
	}
	cleanup, err := InstallFakeGCS(healthy, "da-example")
	if err != nil {
		t.Fatalf("healthy reuse: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}

	unhealthy := func(name string, args ...string) ([]byte, error) {
		return []byte("False"), nil
	}
	if _, err := InstallFakeGCS(unhealthy, "da-example"); err == nil ||
		!strings.Contains(err.Error(), "not healthy") {
		t.Fatalf("unhealthy existing instance: %v", err)
	}

	// A deployment that exists without reporting the condition is unhealthy,
	// not absent.
	blank := func(name string, args ...string) ([]byte, error) {
		return []byte(""), nil
	}
	if _, err := InstallFakeGCS(blank, "da-example"); err == nil ||
		!strings.Contains(err.Error(), "not healthy") {
		t.Fatalf("blank condition: %v", err)
	}
}

func TestInstallFakeGCSRequiresClusterAndPinnedArch(t *testing.T) {
	if _, err := InstallFakeGCS(nil, "  "); err == nil {
		t.Fatal("empty cluster accepted")
	}
	if _, err := fakeGCSImage("riscv64"); err == nil ||
		!strings.Contains(err.Error(), "no pinned digest") {
		t.Fatalf("unpinned arch: %v", err)
	}
}

// The declared endpoint constant matches what the manifest actually serves,
// so a kind values overlay copying FakeGCSEndpoint reaches the container.
func TestFakeGCSEndpointMatchesTheManifest(t *testing.T) {
	if !strings.Contains(fakeGCSKindManifest, "fake-gcs.fake-gcs.svc") {
		t.Error("manifest does not carry the service DNS the endpoint names")
	}
	if !strings.Contains(fakeGCSKindManifest, `"4443"`) &&
		!strings.Contains(fakeGCSKindManifest, "port: 4443") {
		t.Error("manifest does not serve the port the endpoint names")
	}
	if !strings.HasPrefix(FakeGCSEndpoint, "http://fake-gcs.fake-gcs.svc:4443/") {
		t.Errorf("endpoint = %q does not point at the service", FakeGCSEndpoint)
	}
	if !strings.HasSuffix(FakeGCSEndpoint, "/storage/v1/") {
		t.Errorf("endpoint = %q does not name the JSON API base", FakeGCSEndpoint)
	}
}
