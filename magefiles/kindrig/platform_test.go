// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestPlatformKindConfigPinsNodeAndAdmitsIngress(t *testing.T) {
	config := string(platformKindConfig)
	for _, want := range []string{
		"kindest/node:v1.36.1@sha256:",
		`node-labels: "ingress-ready=true"`,
	} {
		if !strings.Contains(config, want) {
			t.Errorf("platform kind config lacks %q", want)
		}
	}
	if strings.Contains(config, "extraPortMappings") {
		t.Error("platform kind config maps host ports; demo clusters own them")
	}
}

func TestEnsurePlatformClusterReplacesLeftover(t *testing.T) {
	kind := &fakeKind{existing: []string{PlatformClusterName}}
	cluster, err := EnsurePlatformCluster(kind.run)
	if err != nil {
		t.Fatal(err)
	}
	if cluster != (Cluster{Name: PlatformClusterName, Created: true}) {
		t.Fatalf("cluster = %+v, want owned %s", cluster, PlatformClusterName)
	}
	var verbs []string
	for _, call := range kind.calls {
		verbs = append(verbs, strings.Join(call[:2], " "))
	}
	if strings.Join(verbs, ",") != "get clusters,delete cluster,create cluster" {
		t.Fatalf("calls = %v, want leftover delete before create", kind.calls)
	}
	create := kind.lastCall("create")
	configPath := create[len(create)-3]
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("staged config %s was not removed: %v", configPath, err)
	}
}

type platformHarness struct {
	kind      *fakeKind
	order     []string
	unbound   bool
	bindErr   error
	bootErr   error
	confErr   error
	evidence  string
	commandFn CommandRunner
}

func (h *platformHarness) options() PlatformOptions {
	return PlatformOptions{
		KindRun: h.kind.run,
		Bind: func(cluster string) (CommandRunner, func(), error) {
			h.order = append(h.order, "bind "+cluster)
			if h.bindErr != nil {
				return nil, nil, h.bindErr
			}
			return h.commandFn, func() { h.unbound = true }, nil
		},
		EvidenceDirectory: h.evidence,
		boot: func(CommandRunner, string) error {
			h.order = append(h.order, "boot")
			return h.bootErr
		},
		conformance: func(CommandRunner, string) error {
			h.order = append(h.order, "conformance")
			return h.confErr
		},
	}
}

func newPlatformHarness(t *testing.T) *platformHarness {
	return &platformHarness{
		kind:      &fakeKind{},
		evidence:  t.TempDir(),
		commandFn: func(string, ...string) ([]byte, error) { return nil, nil },
	}
}

func TestStartPlatformBootsThenChecksThenStopDeletes(t *testing.T) {
	h := newPlatformHarness(t)
	platform, err := StartPlatform(h.options())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.order, ",") != "bind da-platform,boot,conformance" {
		t.Fatalf("order = %v", h.order)
	}
	if h.kind.issued("delete") {
		t.Fatal("successful start deleted the platform")
	}
	platform.Stop(false)
	platform.Stop(false)
	if got := h.kind.lastCall("delete"); strings.Join(got, " ") != "delete cluster --name da-platform" {
		t.Fatalf("stop delete = %v", got)
	}
	if h.kind.issued("export") {
		t.Fatal("successful stop captured failure evidence")
	}
	deletes := 0
	for _, call := range h.kind.calls {
		if call[0] == "delete" {
			deletes++
		}
	}
	if deletes != 1 || !h.unbound {
		t.Fatalf("deletes=%d unbound=%v, want one idempotent delete and unbind", deletes, h.unbound)
	}
}

func TestStartPlatformConformanceFailureCapturesEvidenceAndDeletes(t *testing.T) {
	h := newPlatformHarness(t)
	h.confErr = errors.New("storage provisioner: claim pending")
	platform, err := StartPlatform(h.options())
	if platform != nil || !errors.Is(err, h.confErr) ||
		!strings.Contains(err.Error(), "da-platform platform-conformance") {
		t.Fatalf("start = %v, %v; want conformance failure", platform, err)
	}
	export := h.kind.lastCall("export")
	if export == nil || !strings.HasPrefix(export[2], h.evidence) {
		t.Fatalf("evidence export = %v, want under %s", export, h.evidence)
	}
	if h.kind.lastCall("delete") == nil || !h.unbound {
		t.Fatal("failed conformance left the platform or its kubeconfig behind")
	}
}

func TestStartPlatformBootFailureSkipsConformanceAndDeletes(t *testing.T) {
	h := newPlatformHarness(t)
	h.bootErr = errors.New("traefik rollout timed out")
	if _, err := StartPlatform(h.options()); !errors.Is(err, h.bootErr) ||
		!strings.Contains(err.Error(), "platform-boot") {
		t.Fatalf("start error = %v", err)
	}
	if strings.Join(h.order, ",") != "bind da-platform,boot" {
		t.Fatalf("order = %v, want conformance skipped", h.order)
	}
	if h.kind.lastCall("delete") == nil {
		t.Fatal("boot failure left the platform behind")
	}
}

func TestStartPlatformBindFailureDeletesCluster(t *testing.T) {
	h := newPlatformHarness(t)
	h.bindErr = errors.New("no kubeconfig")
	if _, err := StartPlatform(h.options()); !errors.Is(err, h.bindErr) {
		t.Fatalf("start error = %v", err)
	}
	if h.kind.lastCall("delete") == nil {
		t.Fatal("bind failure left the platform behind")
	}
}

func TestStartPlatformCreateFailureReturnsWithoutPhases(t *testing.T) {
	h := newPlatformHarness(t)
	h.kind.createErr = errors.New("docker out of memory")
	if _, err := StartPlatform(h.options()); err == nil ||
		!strings.Contains(err.Error(), "docker out of memory") {
		t.Fatalf("start error = %v", err)
	}
	if len(h.order) != 0 {
		t.Fatalf("phases ran without a cluster: %v", h.order)
	}
}

func conformanceCluster() *fakeCluster {
	return &fakeCluster{outputs: map[string]string{
		"kubectl get persistentvolumeclaim conformance-claim": "pvc-1234",
	}}
}

func TestPlatformConformanceRunsChecksInOrder(t *testing.T) {
	cluster := conformanceCluster()
	if err := PlatformConformance(cluster.run, PlatformClusterName); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cluster.calls, "\n")
	order := []string{
		"kubectl get --raw=/readyz",
		"kubectl create namespace da-platform-conformance",
		"kubectl apply --namespace da-platform-conformance -f ",
		"kubectl wait --namespace da-platform-conformance --for=jsonpath={.status.phase}=Bound persistentvolumeclaim/conformance-claim",
		"kubectl rollout status deployment/conformance-echo --namespace da-platform-conformance",
		"docker exec da-platform-control-plane curl ",
		"helm uninstall platform-conformance --namespace da-platform-conformance",
		"kubectl wait --for=delete persistentvolume/pvc-1234",
		"kubectl config set-context --current --namespace default",
	}
	position := 0
	for _, want := range order {
		index := strings.Index(joined[position:], want)
		if index < 0 {
			t.Fatalf("missing or out of order %q in:\n%s", want, joined)
		}
		position += index + len(want)
	}
	if !strings.Contains(joined, "--header Host: conformance.localhost http://127.0.0.1/ping") {
		t.Fatalf("route probe does not target the conformance host:\n%s", joined)
	}
}

func TestPlatformConformanceNamesFailedCheckAndCleansUp(t *testing.T) {
	tests := []struct {
		fail string
		want string
	}{
		{"kubectl get --raw=/readyz", "readiness"},
		{"kubectl wait --namespace da-platform-conformance --for=jsonpath", "storage provisioner"},
		{"kubectl rollout status deployment/conformance-echo", "ingress route"},
		{"docker exec", "ingress route"},
		{"kubectl wait --for=delete persistentvolume", "namespace churn"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			cluster := conformanceCluster()
			cluster.fail = map[string]string{test.fail: "check failed"}
			err := PlatformConformance(cluster.run, PlatformClusterName)
			if err == nil || !strings.HasPrefix(err.Error(), test.want) {
				t.Fatalf("error = %v, want prefix %q", err, test.want)
			}
			if test.want == "readiness" {
				return
			}
			joined := strings.Join(cluster.calls, "\n")
			if !strings.Contains(joined, "kubectl delete namespace da-platform-conformance") {
				t.Fatalf("failed check left the conformance namespace:\n%s", joined)
			}
			if !strings.HasSuffix(joined, "kubectl config set-context --current --namespace default") {
				t.Fatalf("failed check did not reset the current namespace:\n%s", joined)
			}
		})
	}
}

func TestPlatformConformanceManifestUsesLoadedImageAndConformanceHost(t *testing.T) {
	for _, want := range []string{
		traefikImagePlaceholder, platformHostPlaceholder,
		"imagePullPolicy: Never", "ingressClassName: traefik",
		"claimName: conformance-claim",
	} {
		if !strings.Contains(platformConformanceManifest, want) {
			t.Errorf("conformance manifest lacks %q", want)
		}
	}
}

func TestAcquirePlatformStartsOwnedPlatformWhenNoneIsListed(t *testing.T) {
	h := newPlatformHarness(t)
	platform, err := AcquirePlatform(h.options())
	if err != nil {
		t.Fatal(err)
	}
	if !platform.Cluster.Created || strings.Join(h.order, ",") != "bind da-platform,boot,conformance" {
		t.Fatalf("platform=%+v order=%v, want an owned, booted, checked platform", platform.Cluster, h.order)
	}
	platform.Stop(false)
	if h.kind.lastCall("delete") == nil {
		t.Fatal("owned platform survived stop")
	}
}

func TestAcquirePlatformReusesRunningPlatformWithoutOwnership(t *testing.T) {
	h := newPlatformHarness(t)
	h.kind.existing = []string{PlatformClusterName}
	checked := &fakeCluster{}
	h.commandFn = checked.run
	platform, err := AcquirePlatform(h.options())
	if err != nil {
		t.Fatal(err)
	}
	if platform.Cluster.Created || strings.Join(h.order, ",") != "bind da-platform" {
		t.Fatalf("platform=%+v order=%v, want unowned reuse without boot or conformance",
			platform.Cluster, h.order)
	}
	if !strings.Contains(strings.Join(checked.calls, "\n"), "kubectl get --raw=/readyz") {
		t.Fatalf("reuse skipped the data-plane check: %v", checked.calls)
	}
	platform.Stop(true)
	if h.kind.issued("create") || h.kind.issued("delete") || h.kind.issued("export") {
		t.Fatalf("reused platform was mutated: %v", h.kind.calls)
	}
	if !h.unbound {
		t.Fatal("reused platform kept its kubeconfig binding")
	}
}

func TestAcquirePlatformRefusesUnreadyRunningPlatform(t *testing.T) {
	h := newPlatformHarness(t)
	h.kind.existing = []string{PlatformClusterName}
	h.commandFn = (&fakeCluster{fail: map[string]string{
		"kubectl get --raw=/readyz": "apiserver down",
	}}).run
	if _, err := AcquirePlatform(h.options()); err == nil ||
		!strings.Contains(err.Error(), "remediation") || !strings.Contains(err.Error(), "apiserver down") {
		t.Fatalf("error = %v, want unready platform refusal", err)
	}
	if h.kind.issued("delete") || !h.unbound {
		t.Fatal("refusal deleted someone else's platform or leaked its binding")
	}
}
