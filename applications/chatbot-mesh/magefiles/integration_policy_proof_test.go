// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

func TestCalicoImagesAreDigestPinnedForSupportedArchitectures(t *testing.T) {
	want := map[string]map[string]string{
		"cni": {
			"amd64": "sha256:3ef9bbb3fdb2b3194dff57d7d8496d5e18247afb59606dfc694ab88ed1fa9f86",
			"arm64": "sha256:f83ba4048763b8dbfa95f65b5094e8fb08b7326ce8d465111bb9da416ecb6bdb",
		},
		"node": {
			"amd64": "sha256:c061070a27292f8152ae6a0582078eb9059d1b6ed5e57c2052e5c22534734240",
			"arm64": "sha256:9da8e32d2d6f9405be1985f258842bfc808bbf5aca51091bdef8110fca722a1b",
		},
		"kube-controllers": {
			"amd64": "sha256:df00967cbd6d88e1ff3123e1598895845622e2987928b4ebd9d8ac49aefe00c3",
			"arm64": "sha256:afa3429708de65af587ede22064a7abddf57082edd368066c24781e3b2d30cb5",
		},
	}
	if len(calicoImages) != len(want) {
		t.Fatalf("Calico image count = %d, want %d", len(calicoImages), len(want))
	}
	for _, image := range calicoImages {
		digests, ok := want[image.Component]
		if !ok {
			t.Fatalf("unexpected Calico component %q", image.Component)
		}
		for _, arch := range []string{"amd64", "arm64"} {
			source, runtimeImage, err := calicoImageRefs(image, arch)
			if err != nil {
				t.Fatalf("%s/%s: %v", image.Component, arch, err)
			}
			if !strings.HasSuffix(source, "@"+digests[arch]) {
				t.Errorf("%s/%s source = %q, want digest %s",
					image.Component, arch, source, digests[arch])
			}
			if source != runtimeImage+"@"+digests[arch] {
				t.Errorf("%s/%s source %q does not pin runtime image %q",
					image.Component, arch, source, runtimeImage)
			}
		}
		if _, _, err := calicoImageRefs(image, "unsupported"); err == nil {
			t.Errorf("%s accepted an architecture without a pinned digest", image.Component)
		}
	}
}

func TestCalicoManifestAndImagesStayOnOneRelease(t *testing.T) {
	if calicoVersion != "v3.32.1" {
		t.Fatalf("Calico version = %q, want v3.32.1", calicoVersion)
	}
	if !strings.Contains(calicoManifest, "/"+calicoManifestCommit+"/manifests/calico.yaml") {
		t.Fatalf("Calico manifest is not pinned to commit %s: %s",
			calicoManifestCommit, calicoManifest)
	}
	if len(calicoManifestCommit) != 40 {
		t.Fatalf("Calico manifest commit = %q, want a full Git commit", calicoManifestCommit)
	}
	for _, image := range calicoImages {
		_, runtimeImage, err := calicoImageRefs(image, "amd64")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(runtimeImage, ":"+calicoVersion) {
			t.Errorf("%s runtime image %q is not aligned with manifest release %s",
				image.Component, runtimeImage, calicoVersion)
		}
	}
}

func TestInstallCalicoPullsTagsAndLoadsEveryImageBeforeApply(t *testing.T) {
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		return nil, nil
	}
	if err := installCalico(run, policyKindCluster, "amd64"); err != nil {
		t.Fatal(err)
	}

	const imageCommandCount = 9
	if len(calls) != imageCommandCount+4 {
		t.Fatalf("Calico install calls = %d, want 13: %v", len(calls), calls)
	}
	for imageIndex, image := range calicoImages {
		source, runtimeImage, err := calicoImageRefs(image, "amd64")
		if err != nil {
			t.Fatal(err)
		}
		base := imageIndex * 3
		want := []string{
			"docker pull --platform linux/amd64 " + source,
			"docker tag " + source + " " + runtimeImage,
			"kind load docker-image " + runtimeImage + " --name " + policyKindCluster,
		}
		for offset := range want {
			if calls[base+offset] != want[offset] {
				t.Errorf("call[%d] = %q, want %q", base+offset, calls[base+offset], want[offset])
			}
		}
	}
	if !strings.Contains(calls[imageCommandCount], "kubectl --context kind-"+
		policyKindCluster+" apply -f "+calicoManifest) {
		t.Fatalf("first post-load command must apply the pinned manifest: %v", calls)
	}
	for index, call := range calls[:imageCommandCount] {
		if strings.Contains(call, "kubectl apply") {
			t.Fatalf("manifest applied before all images were loaded at call %d: %v", index, calls)
		}
	}
	if !strings.Contains(calls[imageCommandCount+2],
		"deployment/calico-kube-controllers") {
		t.Fatalf("kube-controllers rollout was not observed: %v", calls)
	}
}

func TestInstallCalicoErrorsPreserveComponentCommandAndOutput(t *testing.T) {
	commandErr := errors.New("exit status 1")
	tests := []struct {
		name      string
		failMatch string
		component string
		output    string
	}{
		{
			name:      "CNI pull",
			failMatch: "docker pull",
			component: "cni",
			output:    "certificate signed by unknown authority",
		},
		{
			name:      "node load",
			failMatch: "kind load docker-image quay.io/calico/node",
			component: "node",
			output:    "image import failed",
		},
		{
			name:      "controller rollout",
			failMatch: "deployment/calico-kube-controllers",
			component: "kube-controllers rollout",
			output:    "timed out waiting for the condition",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := func(name string, args ...string) ([]byte, error) {
				command := strings.Join(append([]string{name}, args...), " ")
				if strings.Contains(command, test.failMatch) {
					return []byte(test.output), commandErr
				}
				return nil, nil
			}
			err := installCalico(run, policyKindCluster, "amd64")
			if !errors.Is(err, commandErr) {
				t.Fatalf("install error = %v, want wrapped %v", err, commandErr)
			}
			for _, want := range []string{
				calicoVersion,
				test.component,
				test.failMatch,
				test.output,
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("install error omitted %q: %v", want, err)
				}
			}
		})
	}
}

func TestPolicyProbeImageIsDigestPinnedLoadedAndNeverPulledByNode(t *testing.T) {
	wantDigests := map[string]string{
		"amd64": "sha256:b7f3d86d6e84fc17718c48bcde1450807faa2d56704205c697b4bd5df7b9e29f",
		"arm64": "sha256:bd44eb136a95dcc8dc58995e43abc40a413f2e8e3d4a2aae6bccbe94686acb05",
	}
	for arch, digest := range wantDigests {
		source, runtimeImage, err := policyProbeImageRefs(arch)
		if err != nil {
			t.Fatal(err)
		}
		if source != runtimeImage+"@"+digest {
			t.Errorf("%s probe source = %q, want %q", arch, source, runtimeImage+"@"+digest)
		}
	}
	if _, _, err := policyProbeImageRefs("unsupported"); err == nil {
		t.Fatal("probe image accepted an architecture without a pinned digest")
	}

	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		return nil, nil
	}
	if err := preloadPolicyProbeImage(run, policyKindCluster, "amd64"); err != nil {
		t.Fatal(err)
	}
	source, runtimeImage, _ := policyProbeImageRefs("amd64")
	wantCalls := []string{
		"docker pull --platform linux/amd64 " + source,
		"docker tag " + source + " " + runtimeImage,
		"kind load docker-image " + runtimeImage + " --name " + policyKindCluster,
	}
	if strings.Join(calls, "\n") != strings.Join(wantCalls, "\n") {
		t.Fatalf("probe image calls = %v, want %v", calls, wantCalls)
	}

	for name, manifest := range map[string]string{
		"server":  httpdPod("ns", "server", "http", 8080, map[string]string{"role": "server"}),
		"sleeper": sleeperPod("ns", "client", map[string]string{"role": "client"}),
	} {
		if !strings.Contains(manifest, "image: "+runtimeImage) ||
			!strings.Contains(manifest, "imagePullPolicy: Never") {
			t.Errorf("%s pod can pull from the node registry:\n%s", name, manifest)
		}
	}
}

func TestPolicyProbeImageErrorsPreserveCommandAndOutput(t *testing.T) {
	commandErr := errors.New("exit status 1")
	run := func(name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		if strings.HasPrefix(command, "kind load") {
			return []byte("probe import failed"), commandErr
		}
		return nil, nil
	}
	err := preloadPolicyProbeImage(run, policyKindCluster, "arm64")
	if !errors.Is(err, commandErr) {
		t.Fatalf("probe load error = %v, want wrapped %v", err, commandErr)
	}
	for _, want := range []string{
		"policy probe image",
		policyProbeImageRepository + ":" + policyProbeImageVersion,
		"kind load docker-image",
		"probe import failed",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("probe load error omitted %q: %v", want, err)
		}
	}
}

func TestEnsurePolicyClusterReturnsOwnershipWithBootstrapFailure(t *testing.T) {
	t.Chdir("..")
	var kindCalls []string
	kindRun := func(args ...string) ([]byte, error) {
		kindCalls = append(kindCalls, strings.Join(args, " "))
		if len(args) >= 2 && args[0] == "get" && args[1] == "clusters" {
			return nil, nil
		}
		return nil, nil
	}
	bootstrapErr := errors.New("registry unavailable")
	commandRun := func(name string, args ...string) ([]byte, error) {
		return []byte("host pull failed"), bootstrapErr
	}

	cluster, err := ensurePolicyClusterWith(kindRun, commandRun, "amd64")
	if !errors.Is(err, bootstrapErr) {
		t.Fatalf("bootstrap error = %v, want %v", err, bootstrapErr)
	}
	if !cluster.Created || cluster.Name != policyKindCluster {
		t.Fatalf("failed bootstrap returned cluster %+v, want owned %s", cluster, policyKindCluster)
	}
	if len(kindCalls) < 2 || !strings.HasPrefix(kindCalls[1], "create cluster ") {
		t.Fatalf("kind calls = %v, want cluster creation before bootstrap failure", kindCalls)
	}
}

func TestPolicyProofCleanupHonorsOwnershipAndFailure(t *testing.T) {
	bootstrapErr := errors.New("Calico rollout failed")
	proofErr := errors.New("policy assertion failed")
	tests := []struct {
		name       string
		cluster    kindrig.Cluster
		acquireErr error
		proveErr   error
		wantErr    error
		wantExport bool
		wantDelete bool
	}{
		{
			name:       "owned bootstrap failure",
			cluster:    kindrig.Cluster{Name: policyKindCluster, Created: true},
			acquireErr: bootstrapErr,
			wantErr:    bootstrapErr,
			wantExport: true,
			wantDelete: true,
		},
		{
			name:       "reused bootstrap failure",
			cluster:    kindrig.Cluster{Name: policyKindCluster},
			acquireErr: bootstrapErr,
			wantErr:    bootstrapErr,
		},
		{
			name:       "owned proof failure",
			cluster:    kindrig.Cluster{Name: policyKindCluster, Created: true},
			proveErr:   proofErr,
			wantErr:    proofErr,
			wantExport: true,
			wantDelete: true,
		},
		{
			name:       "owned success",
			cluster:    kindrig.Cluster{Name: policyKindCluster, Created: true},
			wantDelete: true,
		},
		{
			name:    "reused success",
			cluster: kindrig.Cluster{Name: policyKindCluster},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sequence []string
			kindRun := func(args ...string) ([]byte, error) {
				sequence = append(sequence, "kind "+strings.Join(args, " "))
				return nil, nil
			}
			commandRun := func(name string, args ...string) ([]byte, error) {
				sequence = append(sequence, name+" "+strings.Join(args, " "))
				return nil, nil
			}
			evidence := kindrig.FailureEvidence{
				Directory:  filepath.Join(t.TempDir(), "evidence"),
				Namespaces: []string{"kube-system"},
				Run:        commandRun,
			}
			proveCalled := false
			err := executePolicyProof("unused-chart", policyProofExecution{
				acquire: func() (kindrig.Cluster, error) {
					return test.cluster, test.acquireErr
				},
				release: func(cluster kindrig.Cluster, failed bool) {
					cluster.ReleaseAfter(kindRun, failed, evidence)
				},
				prove: func(string) error {
					proveCalled = true
					return test.proveErr
				},
			})
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("policy proof = %v, want success", err)
				}
			} else if !errors.Is(err, test.wantErr) {
				t.Fatalf("policy proof error = %v, want %v", err, test.wantErr)
			}
			if got := test.acquireErr == nil; proveCalled != got {
				t.Fatalf("prove called = %v, want %v", proveCalled, got)
			}
			joined := strings.Join(sequence, "\n")
			if got := strings.Contains(joined, "kind export logs"); got != test.wantExport {
				t.Errorf("evidence export = %v, want %v; sequence:\n%s",
					got, test.wantExport, joined)
			}
			if got := strings.Contains(joined, "kind delete cluster"); got != test.wantDelete {
				t.Errorf("cluster delete = %v, want %v; sequence:\n%s",
					got, test.wantDelete, joined)
			}
			if test.wantExport {
				exportIndex := indexContaining(sequence, "kind export logs")
				deleteIndex := indexContaining(sequence, "kind delete cluster")
				if exportIndex < 0 || deleteIndex < 0 || exportIndex >= deleteIndex {
					t.Errorf("failure evidence must precede delete: %v", sequence)
				}
				if !strings.Contains(joined, "kubectl describe all -n kube-system") {
					t.Errorf("kube-system diagnostics were not captured: %v", sequence)
				}
			}
		})
	}
}

func TestPolicyDiagnosticRunnerPinsKubectlContext(t *testing.T) {
	var got string
	run := policyDiagnosticRunner(func(name string, args ...string) ([]byte, error) {
		got = strings.Join(append([]string{name}, args...), " ")
		return nil, nil
	})
	if _, err := run("kubectl", "get", "pods", "-n", "kube-system"); err != nil {
		t.Fatal(err)
	}
	want := "kubectl --context " + policyKubeContext() + " get pods -n kube-system"
	if got != want {
		t.Fatalf("diagnostic command = %q, want %q", got, want)
	}
}

func indexContaining(values []string, fragment string) int {
	for index, value := range values {
		if strings.Contains(value, fragment) {
			return index
		}
	}
	return -1
}

func TestPolicyFailureEvidenceIncludesKubeSystem(t *testing.T) {
	evidence := policyFailureEvidence(
		filepath.Join(t.TempDir(), "helm"),
		func(string, ...string) ([]byte, error) { return nil, nil },
	)
	found := false
	for _, namespace := range evidence.Namespaces {
		found = found || namespace == "kube-system"
	}
	if !found {
		t.Fatal("policy failure evidence must include kube-system")
	}
	if !strings.Contains(evidence.Directory,
		filepath.Join("build", "kind-evidence", policyKindCluster+"-")) {
		t.Fatalf("policy evidence directory = %q, want persistent build/kind-evidence path",
			evidence.Directory)
	}
}
