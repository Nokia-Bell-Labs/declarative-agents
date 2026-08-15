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

func TestDemoUpRetainsSuccessfulOwnedCluster(t *testing.T) {
	kind := &fakeKind{}
	deployed := false
	err := DemoUp(kind.run, "da-example-demo", testConfig(t), 0, func(cluster Cluster) error {
		deployed = cluster.Created
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !deployed || kind.issued("delete") {
		t.Fatalf("deployed=%v calls=%v", deployed, kind.calls)
	}
}

func TestDemoUpFailureReleasesOnlyOwnedCluster(t *testing.T) {
	deployErr := errors.New("helm failed")
	tests := []struct {
		name       string
		existing   []string
		wantDelete bool
	}{
		{name: "owned", wantDelete: true},
		{name: "reused", existing: []string{"da-example-demo"}, wantDelete: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := &fakeKind{
				existing:   test.existing,
				kubeconfig: []byte("apiVersion: v1\nkind: Config\n"),
			}
			err := demoUp(kind.run, "da-example-demo", testConfig(t), 0,
				healthyEnsureOptions(), func(Cluster) error { return deployErr })
			if !errors.Is(err, deployErr) {
				t.Fatalf("error = %v", err)
			}
			if got := kind.issued("delete"); got != test.wantDelete {
				t.Fatalf("delete=%v want=%v calls=%v", got, test.wantDelete, kind.calls)
			}
		})
	}
}

func TestDemoDownDeletesOnlyNamedDemoCluster(t *testing.T) {
	kind := &fakeKind{existing: []string{"developer", "da-example-demo"}}
	if err := DemoDown(kind.run, "da-example-demo"); err != nil {
		t.Fatal(err)
	}
	deleteCall := kind.lastCall("delete")
	if strings.Join(deleteCall, " ") != "delete cluster --name da-example-demo" {
		t.Fatalf("delete call = %v", deleteCall)
	}
}

func TestTraefikImageIsPinnedPerSupportedArchitecture(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		image, err := traefikImage(arch)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"docker.io/library/traefik:v3.7.10@sha256:",
			traefikImageDigests[arch],
		} {
			if !strings.Contains(image, want) {
				t.Errorf("%s image %q missing %q", arch, image, want)
			}
		}
	}
	if _, err := traefikImage("unsupported"); err == nil {
		t.Fatal("unsupported architecture accepted without a pinned digest")
	}
}

func TestTraefikManifestIsMinimalStandardIngressController(t *testing.T) {
	for _, want := range []string{
		"kind: IngressClass",
		"name: traefik",
		"controller: traefik.io/ingress-controller",
		"resources: [services, endpoints, secrets, nodes]",
		"--providers.kubernetesingress=true",
		"--providers.kubernetesingress.ingressclass=traefik",
		"hostPort: 80",
		"hostPort: 443",
		"imagePullPolicy: Never",
		"requests: {cpu: 25m, memory: 64Mi}",
		"limits: {cpu: 500m, memory: 128Mi}",
		"readinessProbe:",
		"path: /ping",
		traefikImagePlaceholder,
	} {
		if !strings.Contains(traefikKindManifest, want) {
			t.Errorf("Traefik manifest missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"ValidatingWebhookConfiguration",
		"MutatingWebhookConfiguration",
		"kind: IngressRoute",
		"--providers.kubernetescrd=true",
		"--providers.kubernetesgateway=true",
		"--api.dashboard=true",
	} {
		if strings.Contains(traefikKindManifest, forbidden) {
			t.Errorf("minimal Traefik manifest contains %q", forbidden)
		}
	}
}

func TestInstallIngressLoadsPinnedImageAndWaitsForDeployment(t *testing.T) {
	var calls []string
	var applied string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		if name == "kubectl" && len(args) == 3 && args[0] == "apply" {
			data, err := os.ReadFile(args[2])
			if err != nil {
				return nil, err
			}
			applied = string(data)
		}
		return nil, nil
	}
	if err := InstallIngress(run, "da-example-demo"); err != nil {
		t.Fatal(err)
	}
	image, err := traefikImage(runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	runtimeImage := traefikRuntimeRepository + ":" + traefikImageVersion
	wantCalls := []string{
		"docker pull --platform linux/" + runtime.GOARCH + " " + image,
		"docker tag " + image + " " + runtimeImage,
		"kind load docker-image " + runtimeImage + " --name da-example-demo",
		"kubectl apply -f ",
		"kubectl rollout status deployment/traefik --namespace traefik --timeout=180s",
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("calls=%v want %d", calls, len(wantCalls))
	}
	for i, want := range wantCalls {
		if !strings.Contains(calls[i], want) {
			t.Errorf("call[%d]=%q missing %q", i, calls[i], want)
		}
	}
	if !strings.Contains(applied, "image: "+runtimeImage) {
		t.Errorf("applied manifest does not carry kind-loaded image %q", runtimeImage)
	}
	if strings.Contains(applied, traefikImagePlaceholder) {
		t.Fatal("applied manifest retains image placeholder")
	}
}

func TestInstallIngressLive(t *testing.T) {
	cluster := os.Getenv("KINDRIG_TRAEFIK_LIVE_CLUSTER")
	if cluster == "" {
		t.Skip("set KINDRIG_TRAEFIK_LIVE_CLUSTER to an existing kind cluster")
	}
	if err := InstallIngress(DefaultCommandRun, cluster); err != nil {
		t.Fatal(err)
	}
}
