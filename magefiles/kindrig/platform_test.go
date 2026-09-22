// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPlatformKindConfigPinsNodeAndAdmitsIngress(t *testing.T) {
	config := string(platformKindConfig)
	for _, want := range []string{
		"kindest/node:v1.36.1@sha256:",
		`node-labels: "ingress-ready=true"`,
		// The platform owns host 80/443 so applications declare .localhost
		// hosts and never coordinate ports (#2477 R2).
		"containerPort: 80, hostPort: 80",
		"containerPort: 443, hostPort: 443",
		// The stable object-store mount backs durable local telemetry (#2477 R3).
		platformObjectsPlaceholder,
	} {
		if !strings.Contains(config, want) {
			t.Errorf("platform kind config lacks %q", want)
		}
	}
}

// #2477 R1: fake-GCS is a platform boot service, installed by BootPlatform
// alongside ingress and metrics-server.
func TestBootPlatformInstallsFakeGCS(t *testing.T) {
	config := string(fakeGCSKindManifest)
	for _, want := range []string{"-backend", "filesystem", "-filesystem-root", "hostPath", "/var/lib/da-platform-objects"} {
		if !strings.Contains(config, want) {
			t.Errorf("fake-gcs manifest lacks %q; the backend must be durable filesystem", want)
		}
	}
	if strings.Contains(config, "- memory") {
		t.Error("fake-gcs manifest still uses the memory backend arg; #2477 R3 requires filesystem durability")
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

func TestPlatformKindConfigExportsControlPlaneTracing(t *testing.T) {
	config := string(platformKindConfig)
	for _, want := range []string{
		"hostPath: " + platformTracingPlaceholder,
		"containerPath: /etc/kubernetes/tracing.yaml",
		"tracing-config-file", "OTEL_RESOURCE_ATTRIBUTES",
		"kind: KubeletConfiguration", "endpoint: host.docker.internal:4317",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("platform kind config lacks %q", want)
		}
	}
	if !strings.Contains(string(platformTracingConfig), "endpoint: host.docker.internal:4317") {
		t.Error("platform tracing config does not target host observability")
	}
	path, err := stagePlatformTracingConfig()
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(platformTracingConfig) {
		t.Fatalf("staged tracing config at %s: %v", path, err)
	}
}

func TestPlatformDetachHandsOverClusterWithoutDeleting(t *testing.T) {
	h := newPlatformHarness(t)
	platform, err := StartPlatform(h.options())
	if err != nil {
		t.Fatal(err)
	}
	cluster := platform.Detach()
	platform.Stop(true)
	if cluster != (Cluster{Name: PlatformClusterName, Created: true}) || !h.unbound {
		t.Fatalf("detached cluster=%+v unbound=%v", cluster, h.unbound)
	}
	if h.kind.issued("delete") {
		t.Fatal("detach or a later stop deleted the platform")
	}
}

func listedPlatformHarness(t *testing.T) (*platformHarness, *fakeCluster) {
	h := newPlatformHarness(t)
	h.kind.existing = []string{PlatformClusterName}
	h.kind.kubeconfig = []byte("apiVersion: v1\nkind: Config\n")
	commands := &fakeCluster{}
	h.commandFn = commands.run
	return h, commands
}

func (h *platformHarness) upOptions(healthy bool) PlatformOptions {
	options := h.options()
	options.healthRun = func(string, ...string) ([]byte, error) {
		if healthy {
			return []byte("ok"), nil
		}
		return []byte("connection refused"), errors.New("unhealthy")
	}
	return options
}

func TestUpPlatformStartsAndKeepsPlatformWhenNoneIsListed(t *testing.T) {
	h := newPlatformHarness(t)
	cluster, err := UpPlatform(h.upOptions(true))
	if err != nil {
		t.Fatal(err)
	}
	if !cluster.Created || strings.Join(h.order, ",") != "bind da-platform,boot,conformance" {
		t.Fatalf("cluster=%+v order=%v", cluster, h.order)
	}
	if h.kind.issued("delete") || !h.unbound {
		t.Fatal("platform:up deleted its platform or kept the kubeconfig binding")
	}
}

func TestUpPlatformReusesHealthyPlatformAndPrunesNodeImages(t *testing.T) {
	h, commands := listedPlatformHarness(t)
	cluster, err := UpPlatform(h.upOptions(true))
	if err != nil {
		t.Fatal(err)
	}
	if cluster.Created || h.kind.issued("create") || h.kind.issued("delete") {
		t.Fatalf("reuse recreated or deleted the platform: cluster=%+v calls=%v", cluster, h.kind.calls)
	}
	if strings.Join(h.order, ",") != "bind da-platform,boot,conformance" || !h.unbound {
		t.Fatalf("order=%v unbound=%v", h.order, h.unbound)
	}
	if strings.Join(commands.calls, "\n") != "docker exec da-platform-control-plane crictl rmi --prune" {
		t.Fatalf("reuse did not prune node images first: %v", commands.calls)
	}
}

func TestUpPlatformRefusesUnhealthyPlatformWithoutDeleting(t *testing.T) {
	h, _ := listedPlatformHarness(t)
	if _, err := UpPlatform(h.upOptions(false)); err == nil ||
		!strings.Contains(err.Error(), "refusing to delete") {
		t.Fatalf("error = %v, want unhealthy refusal", err)
	}
	if h.kind.issued("delete") || h.kind.issued("create") || len(h.order) != 0 {
		t.Fatalf("unhealthy platform was mutated: calls=%v order=%v", h.kind.calls, h.order)
	}
}

func TestUpPlatformReuseConformanceFailureKeepsPlatform(t *testing.T) {
	h, _ := listedPlatformHarness(t)
	h.confErr = errors.New("ingress route: 404")
	if _, err := UpPlatform(h.upOptions(true)); !errors.Is(err, h.confErr) {
		t.Fatalf("error = %v, want conformance failure", err)
	}
	if h.kind.issued("delete") {
		t.Fatal("a failed reuse deleted the developer's platform")
	}
}

// noManagedNamespaces binds a command runner that reports no managed
// application namespaces, so DownPlatform proceeds to delete the cluster.
func noManagedNamespaces(string) (CommandRunner, func(), error) {
	return func(string, ...string) ([]byte, error) { return []byte(""), nil }, func() {}, nil
}

func TestDownPlatformDeletesOnlyThePlatform(t *testing.T) {
	kind := &fakeKind{existing: []string{"da-chatbot-mesh-demo", PlatformClusterName}}
	if err := DownPlatform(kind.run, noManagedNamespaces); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(kind.lastCall("delete"), " "); got != "delete cluster --name da-platform" {
		t.Fatalf("delete = %q", got)
	}
	absent := &fakeKind{existing: []string{"da-chatbot-mesh-demo"}}
	if err := DownPlatform(absent.run, noManagedNamespaces); err != nil || absent.issued("delete") {
		t.Fatalf("absent platform: err=%v calls=%v", err, absent.calls)
	}
}

// #2477 R7: platform:down refuses while a managed application namespace remains
// and does not delete the cluster.
func TestDownPlatformRefusesWhileApplicationsRemain(t *testing.T) {
	kind := &fakeKind{existing: []string{PlatformClusterName}}
	bind := func(string) (CommandRunner, func(), error) {
		return func(string, ...string) ([]byte, error) {
			return []byte("da-chatbot-mesh da-coding-agent"), nil
		}, func() {}, nil
	}
	err := DownPlatform(kind.run, bind)
	if err == nil || !strings.Contains(err.Error(), "da-chatbot-mesh") {
		t.Fatalf("down did not refuse naming the application namespace: %v", err)
	}
	if kind.issued("delete") {
		t.Fatal("down deleted the platform while an application namespace remained")
	}
}

// #2477 R6: platform:reset refuses while da-platform is running rather than
// deleting retained data as a side effect.
func TestResetPlatformRefusesWhileRunning(t *testing.T) {
	kind := &fakeKind{existing: []string{PlatformClusterName}}
	err := ResetPlatform(kind.run, noManagedNamespaces)
	if err == nil || !strings.Contains(err.Error(), "refuses while") {
		t.Fatalf("reset did not refuse while the platform was running: %v", err)
	}
}

// #2477 R10: preflight refuses when a process or demo cluster holds a platform
// host port, naming the port and the remediation; it passes when both are free.
func TestPlatformPreflightDetectsHeldHostPorts(t *testing.T) {
	if err := PlatformPreflight(func(int) bool { return false }); err != nil {
		t.Fatalf("preflight refused with both ports free: %v", err)
	}
	held := PlatformPreflight(func(port int) bool { return port == 443 })
	if held == nil {
		t.Fatal("preflight passed while host 443 was held")
	}
	if !strings.Contains(held.Error(), "443") || !strings.Contains(held.Error(), "kind delete cluster") {
		t.Fatalf("preflight error names neither the port nor the remediation: %v", held)
	}
}

// #2477 R10: StartPlatform runs preflight before creating the cluster, so a
// held port refuses without touching kind.
func TestStartPlatformPreflightsBeforeCreate(t *testing.T) {
	kind := &fakeKind{}
	_, err := StartPlatform(PlatformOptions{
		KindRun:   kind.run,
		PortProbe: func(int) bool { return true },
	})
	if err == nil || !strings.Contains(err.Error(), "preflight") {
		t.Fatalf("StartPlatform did not preflight: %v", err)
	}
	if kind.issued("create") {
		t.Fatal("StartPlatform created a cluster despite a held host port")
	}
}

func TestBoundedCommandRunnerFailsAStalledCommand(t *testing.T) {
	stalled := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	run := boundedCommandRunner(stalled, 20*time.Millisecond)
	started := time.Now()
	_, err := run("docker", "pull", "example@sha256:abc")
	if err == nil || !strings.Contains(err.Error(), "docker pull example@sha256:abc") ||
		!strings.Contains(err.Error(), "no result within 20ms") {
		t.Fatalf("error = %v, want the stalled command and its bound named", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded runner waited %s", elapsed)
	}
}

func TestBoundedCommandRunnerPassesThroughResults(t *testing.T) {
	want := errors.New("exit status 1")
	run := boundedCommandRunner(func(context.Context, string, ...string) ([]byte, error) {
		return []byte("output"), want
	}, time.Minute)
	if output, err := run("kubectl", "get", "ns"); !errors.Is(err, want) || string(output) != "output" {
		t.Fatalf("output=%q err=%v, want the command's own result", output, err)
	}
}

func TestPlatformDefaultsBoundEveryCommand(t *testing.T) {
	if got := (PlatformOptions{}).withDefaults().commandTimeout; got != platformCommandTimeout {
		t.Fatalf("default command timeout = %s, want %s", got, platformCommandTimeout)
	}
}
