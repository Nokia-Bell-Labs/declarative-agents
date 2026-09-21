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
		if strings.HasPrefix(command, "kubectl --context kind-da-example get deployment") {
			return []byte(`Error from server (NotFound): deployments.apps "fake-gcs" not found`), errors.New("NotFound")
		}
		if name == "kubectl" && appliedManifestPath(args) != "" {
			data, err := os.ReadFile(appliedManifestPath(args))
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
		"kubectl --context kind-da-example get deployment " + fakeGCSDeployment,
		"docker image inspect --format {{.Id}} " + source,
		"docker tag " + source + " " + runtimeImage,
		"node-import " + runtimeImage + " da-example-control-plane linux/" + runtime.GOARCH,
		"kubectl --context kind-da-example apply -f ",
		"kubectl --context kind-da-example rollout status deployment/" + fakeGCSDeployment +
			" --namespace " + fakeGCSNamespace + " --timeout=180s",
		"kubectl --context kind-da-example delete -f ",
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

// appliedManifestPath returns the -f path of a kubectl apply, wherever the
// flag sits: binding a command to its cluster context (GH-2428) shifts every
// positional argument, and a recorder that reads args[2] silently stops
// seeing the manifest.
func appliedManifestPath(args []string) string {
	apply := false
	for index, arg := range args {
		if arg == "apply" {
			apply = true
		}
		if apply && arg == "-f" && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

// GH-2428: every kubectl an installer runs is bound to the cluster it loads
// images into. Unbound, a stale context sends the manifest to one cluster
// while the image lands in another, and the rollout times out naming
// neither.
func TestInstallersBindEveryKubectlToTheirCluster(t *testing.T) {
	for name, install := range map[string]func(CommandRunner, string) error{
		"fake-gcs": func(run CommandRunner, cluster string) error {
			_, err := InstallFakeGCS(run, cluster)
			return err
		},
		"metrics-server": func(run CommandRunner, cluster string) error {
			_, err := InstallMetricsServer(run, cluster)
			return err
		},
		"ingress": InstallIngress,
	} {
		t.Run(name, func(t *testing.T) {
			var unbound []string
			run := func(command string, args ...string) ([]byte, error) {
				if command == "kubectl" {
					joined := strings.Join(args, " ")
					if !strings.HasPrefix(joined, "--context kind-da-target ") {
						unbound = append(unbound, "kubectl "+joined)
					}
				}
				if command == "kubectl" && strings.Contains(strings.Join(args, " "), "get ") {
					return []byte("NotFound"), errors.New("NotFound")
				}
				return nil, nil
			}
			_ = install(run, "da-target")
			if len(unbound) > 0 {
				t.Errorf("%s ran kubectl against the ambient context:\n  %s",
					name, strings.Join(unbound, "\n  "))
			}
		})
	}
}

// A non-kubectl command is never rewritten, and an empty cluster leaves the
// command alone rather than producing a context named "kind-".
func TestInClusterLeavesOtherCommandsAlone(t *testing.T) {
	docker := []string{"docker", "tag", "a", "b"}
	if got := inCluster("da-example", docker); len(got) != len(docker) {
		t.Errorf("docker command rewritten: %v", got)
	}
	kubectl := []string{"kubectl", "get", "pods"}
	if got := inCluster("  ", kubectl); len(got) != len(kubectl) {
		t.Errorf("empty cluster produced a context: %v", got)
	}
	if got := inCluster("da-example", kubectl); got[1] != "--context" || got[2] != "kind-da-example" {
		t.Errorf("binding = %v", got)
	}
}
