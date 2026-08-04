// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeKind records the kind subcommands a run issues and replays a scripted
// cluster list, so ownership is proven without creating a real cluster.
type fakeKind struct {
	existing      []string
	calls         [][]string
	createErr     error
	deleteErr     error
	listErr       error
	exportErr     error
	kubeconfig    []byte
	kubeconfigErr error
}

func (f *fakeKind) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case len(args) >= 2 && args[0] == "get" && args[1] == "clusters":
		if f.listErr != nil {
			return nil, f.listErr
		}
		return []byte(strings.Join(f.existing, "\n") + "\n"), nil
	case len(args) >= 2 && args[0] == "get" && args[1] == "kubeconfig":
		if f.kubeconfigErr != nil {
			return f.kubeconfig, f.kubeconfigErr
		}
		return f.kubeconfig, nil
	case len(args) >= 2 && args[0] == "create":
		return nil, f.createErr
	case len(args) >= 2 && args[0] == "delete":
		return nil, f.deleteErr
	case len(args) >= 2 && args[0] == "export":
		return nil, f.exportErr
	}
	return nil, nil
}

func (f *fakeKind) issued(verb string) bool {
	for _, call := range f.calls {
		if len(call) > 0 && call[0] == verb {
			return true
		}
	}
	return false
}

func (f *fakeKind) lastCall(verb string) []string {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if len(f.calls[i]) > 0 && f.calls[i][0] == verb {
			return f.calls[i]
		}
	}
	return nil
}

// testConfig writes a minimal kind config file and returns its path, standing
// in for the checked-in per-scenario configuration eng01 requires.
func testConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kind-config.yaml")
	content := "kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}

func healthyEnsureOptions() EnsureOptions {
	return EnsureOptions{
		HealthRun: func(string, ...string) ([]byte, error) {
			return []byte("ok"), nil
		},
	}
}

// TestEnsureClusterCreatesWhenAbsent covers the absent case: no cluster
// exists, so the run creates one and owns it.
func TestEnsureClusterCreatesWhenAbsent(t *testing.T) {
	kind := &fakeKind{existing: []string{"some-other-cluster"}}
	cluster, err := EnsureCluster(kind.run, "da-chatbot-mesh-smoke", testConfig(t), 120*time.Second)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !cluster.Created {
		t.Error("a cluster this run created must be owned")
	}
	if !kind.issued("create") {
		t.Error("expected a create call")
	}
}

// TestEnsureClusterReusesPreExistingWithoutOwnership covers the pre-existing
// case: the run reuses the cluster and must not claim ownership.
func TestEnsureClusterReusesPreExistingWithoutOwnership(t *testing.T) {
	kind := &fakeKind{
		existing:   []string{"da-chatbot-mesh-smoke"},
		kubeconfig: []byte("apiVersion: v1\nkind: Config\n"),
	}
	for i := 0; i < 2; i++ {
		cluster, err := EnsureClusterWithOptions(
			kind.run, "da-chatbot-mesh-smoke", testConfig(t), 120*time.Second,
			healthyEnsureOptions())
		if err != nil {
			t.Fatalf("ensure %d: %v", i+1, err)
		}
		if cluster.Created {
			t.Error("a healthy pre-existing cluster must never be owned by this run")
		}
	}
	if kind.issued("create") || kind.issued("delete") {
		t.Errorf("healthy reuse must not mutate the cluster, calls: %v", kind.calls)
	}
}

func TestEnsureClusterRefusesUnhealthyUnownedReuse(t *testing.T) {
	kind := &fakeKind{
		existing:   []string{"developer-cluster"},
		kubeconfig: []byte("apiVersion: v1\nkind: Config\n"),
	}
	var healthCall string
	var generatedKubeconfig string
	options := EnsureOptions{
		HealthRun: func(name string, args ...string) ([]byte, error) {
			healthCall = name + " " + strings.Join(args, " ")
			generatedKubeconfig = args[1]
			content, err := os.ReadFile(generatedKubeconfig)
			if err != nil {
				t.Fatalf("read generated health kubeconfig: %v", err)
			}
			if !strings.Contains(string(content), "kind: Config") {
				t.Fatalf("health kubeconfig = %q, want generated cluster config", content)
			}
			return []byte("TLS handshake timeout"), errors.New("exit status 1")
		},
	}
	cluster, err := EnsureClusterWithOptions(
		kind.run, "developer-cluster", testConfig(t), 120*time.Second, options)
	if err == nil {
		t.Fatal("an unhealthy unowned cluster must be refused")
	}
	if cluster != (Cluster{}) {
		t.Fatalf("refused cluster = %+v, want zero ownership", cluster)
	}
	for _, want := range []string{
		"developer-cluster",
		"health command kubectl",
		"get --raw=/readyz",
		"refusing to delete",
		"remediation:",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error omitted %q: %v", want, err)
		}
	}
	if !strings.Contains(healthCall, "--kubeconfig ") ||
		!strings.Contains(healthCall, "--request-timeout=10s get --raw=/readyz") {
		t.Errorf("health call = %q, want generated kubeconfig and readyz", healthCall)
	}
	if kind.issued("delete") || kind.issued("create") {
		t.Fatalf("unowned cluster was mutated: %v", kind.calls)
	}
	if _, statErr := os.Stat(generatedKubeconfig); !os.IsNotExist(statErr) {
		t.Fatalf("health kubeconfig was not cleaned up: %v", statErr)
	}
}

func TestEnsureClusterRecreatesUnhealthyOwnedReuse(t *testing.T) {
	kind := &fakeKind{
		existing:   []string{"da-coding-agent-applier"},
		kubeconfig: []byte("apiVersion: v1\nkind: Config\n"),
	}
	options := EnsureOptions{
		ReusePolicy: RecreateUnhealthyOwnedCluster,
		HealthRun: func(string, ...string) ([]byte, error) {
			return []byte("connection refused"), errors.New("exit status 1")
		},
	}
	cluster, err := EnsureClusterWithOptions(
		kind.run, "da-coding-agent-applier", testConfig(t), 120*time.Second, options)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !cluster.Created {
		t.Fatal("a recreated cluster must be owned for cleanup")
	}
	if len(kind.calls) != 4 ||
		kind.calls[0][0] != "get" ||
		kind.calls[1][1] != "kubeconfig" ||
		kind.calls[2][0] != "delete" ||
		kind.calls[3][0] != "create" {
		t.Fatalf("recovery calls = %v, want list, kubeconfig, delete, create", kind.calls)
	}

	cluster.Release(kind.run)
	if deletes := countKindCalls(kind.calls, "delete"); deletes != 2 {
		t.Fatalf("cleanup delete count = %d, want recovery plus owned release; calls: %v",
			deletes, kind.calls)
	}
}

func TestEnsureClusterReportsOwnedRecreationFailureWithoutOwnership(t *testing.T) {
	createErr := errors.New("node image unavailable")
	kind := &fakeKind{
		existing:   []string{"da-coding-agent-applier"},
		kubeconfig: []byte("apiVersion: v1\nkind: Config\n"),
		createErr:  createErr,
	}
	options := EnsureOptions{
		ReusePolicy: RecreateUnhealthyOwnedCluster,
		HealthRun: func(string, ...string) ([]byte, error) {
			return nil, errors.New("API unavailable")
		},
	}
	cluster, err := EnsureClusterWithOptions(
		kind.run, "da-coding-agent-applier", testConfig(t), 120*time.Second, options)
	if !errors.Is(err, createErr) {
		t.Fatalf("recreation error = %v, want %v", err, createErr)
	}
	for _, want := range []string{
		"da-coding-agent-applier", "health command kubectl", "remediation:",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("recreation error omitted %q: %v", want, err)
		}
	}
	if cluster != (Cluster{}) {
		t.Fatalf("failed recreation claimed ownership: %+v", cluster)
	}
	if countKindCalls(kind.calls, "delete") != 1 || !kind.issued("create") {
		t.Fatalf("failed recreation calls = %v, want delete then create", kind.calls)
	}
	cluster.Release(kind.run)
	if countKindCalls(kind.calls, "delete") != 1 {
		t.Fatalf("zero result attempted cleanup: %v", kind.calls)
	}
}

func countKindCalls(calls [][]string, verb string) int {
	var count int
	for _, call := range calls {
		if len(call) > 0 && call[0] == verb {
			count++
		}
	}
	return count
}

// TestEnsureClusterRequiresConfig is the eng01 gate: creating a cluster
// without a checked-in configuration file is an error, before any kind
// subcommand is issued.
func TestEnsureClusterRequiresConfig(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
	}{
		{"empty path", ""},
		{"missing file", filepath.Join(t.TempDir(), "absent.yaml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind := &fakeKind{}
			if _, err := EnsureCluster(kind.run, "da-chatbot-mesh-smoke", tt.configPath, 0); err == nil {
				t.Fatal("ensure without a config file must be an error")
			}
			if len(kind.calls) != 0 {
				t.Errorf("no kind subcommand may run without a config, got %v", kind.calls)
			}
		})
	}
}

// TestEnsureClusterPassesConfigAndWait pins the create invocation: the config
// path always rides along, and the readiness wait appears only when the caller
// asked for one (a CNI-less scenario must create without a wait, because its
// node cannot become Ready until the CNI is installed after create).
func TestEnsureClusterPassesConfigAndWait(t *testing.T) {
	config := testConfig(t)
	tests := []struct {
		name     string
		wait     time.Duration
		wantWait bool
	}{
		{"with wait", 120 * time.Second, true},
		{"without wait", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind := &fakeKind{}
			if _, err := EnsureCluster(kind.run, "da-chatbot-mesh-policy", config, tt.wait); err != nil {
				t.Fatalf("ensure: %v", err)
			}
			create := kind.lastCall("create")
			if create == nil {
				t.Fatal("expected a create call")
			}
			joined := strings.Join(create, " ")
			if !strings.Contains(joined, "--config "+config) {
				t.Errorf("create must pass the config file, got %v", create)
			}
			if got := strings.Contains(joined, "--wait"); got != tt.wantWait {
				t.Errorf("create --wait present = %v, want %v (%v)", got, tt.wantWait, create)
			}
		})
	}
}

// TestClusterReleaseDeletesOnlyWhatThisRunCreated is the regression guard for
// GH-589: releasing a reused cluster must issue no delete, so an integration
// run cannot destroy a developer or CI cluster it did not create.
func TestClusterReleaseDeletesOnlyWhatThisRunCreated(t *testing.T) {
	tests := []struct {
		name       string
		cluster    Cluster
		wantDelete bool
	}{
		{"created by this run", Cluster{Name: "da-chatbot-mesh-smoke", Created: true}, true},
		{"pre-existing", Cluster{Name: "da-chatbot-mesh-smoke"}, false},
		{"never acquired", Cluster{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind := &fakeKind{}
			tt.cluster.Release(kind.run)
			if got := kind.issued("delete"); got != tt.wantDelete {
				t.Errorf("delete issued = %v, want %v", got, tt.wantDelete)
			}
		})
	}
}

// TestEnsureClusterReportsCreateFailure covers the create-failed case: the
// error propagates and nothing is claimed as owned, so no delete follows.
func TestEnsureClusterReportsCreateFailure(t *testing.T) {
	kind := &fakeKind{createErr: fmt.Errorf("exit status 1")}
	cluster, err := EnsureCluster(kind.run, "da-chatbot-mesh-smoke", testConfig(t), 120*time.Second)
	if err == nil {
		t.Fatal("a failed create must be reported")
	}
	if cluster.Created {
		t.Error("a failed create must not be owned")
	}
	cluster.Release(kind.run)
	if kind.issued("delete") {
		t.Error("a cluster that was never created must not be deleted")
	}
}

// TestClusterReleaseToleratesCleanupFailure covers the cleanup-failed case: a
// delete error is reported but does not panic or block the caller's own result.
func TestClusterReleaseToleratesCleanupFailure(t *testing.T) {
	kind := &fakeKind{deleteErr: fmt.Errorf("exit status 1")}
	Cluster{Name: "da-chatbot-mesh-smoke", Created: true}.Release(kind.run)
	if !kind.issued("delete") {
		t.Error("an owned cluster must still attempt deletion")
	}
}

// TestExistsTreatsListFailureAsAbsent asserts an unreadable cluster list does
// not report a cluster as pre-existing. Ensure then attempts a create, whose
// own error surfaces, rather than silently reusing an unknown cluster.
func TestExistsTreatsListFailureAsAbsent(t *testing.T) {
	kind := &fakeKind{listErr: fmt.Errorf("kind not on PATH")}
	if Exists(kind.run, "da-chatbot-mesh-smoke") {
		t.Error("an unreadable cluster list must not report the cluster as present")
	}
}

// TestExportLogs pins the export invocation and error propagation, so a failed
// run's evidence step cannot silently do nothing.
func TestExportLogs(t *testing.T) {
	kind := &fakeKind{}
	if err := ExportLogs(kind.run, "da-chatbot-mesh-smoke", "/tmp/evidence"); err != nil {
		t.Fatalf("export: %v", err)
	}
	export := kind.lastCall("export")
	want := []string{"export", "logs", "/tmp/evidence", "--name", "da-chatbot-mesh-smoke"}
	if strings.Join(export, " ") != strings.Join(want, " ") {
		t.Errorf("export call = %v, want %v", export, want)
	}

	failing := &fakeKind{exportErr: fmt.Errorf("exit status 1")}
	if err := ExportLogs(failing.run, "da-chatbot-mesh-smoke", "/tmp/evidence"); err == nil {
		t.Error("a failed export must be reported")
	}
}

func TestLoadImageUsesInjectedRunner(t *testing.T) {
	var call []string
	run := func(_ context.Context, args ...string) ([]byte, error) {
		call = append([]string(nil), args...)
		return []byte("loaded"), nil
	}
	if err := LoadImage(
		context.Background(), run, "da-chatbot-mesh-smoke", "agent-core:dev"); err != nil {
		t.Fatalf("load image: %v", err)
	}
	want := []string{
		"load", "docker-image", "agent-core:dev", "--name", "da-chatbot-mesh-smoke",
	}
	if strings.Join(call, " ") != strings.Join(want, " ") {
		t.Fatalf("load call = %v, want %v", call, want)
	}
}

func TestLoadImageReportsContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	run := func(ctx context.Context, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return []byte("still importing"), errors.New("runner stopped")
	}
	err := LoadImage(ctx, run, "da-chatbot-mesh-smoke", "agent-core:dev")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("load timeout = %v, want context deadline exceeded", err)
	}
	if !strings.Contains(err.Error(), "still importing") {
		t.Fatalf("load timeout omitted command output: %v", err)
	}
}

func TestLoadImageReportsCommandFailure(t *testing.T) {
	commandErr := errors.New("exit status 1")
	run := func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("image not present locally"), commandErr
	}
	err := LoadImage(
		context.Background(), run, "da-coding-agent-smoke", "coding-agent:dev")
	if !errors.Is(err, commandErr) {
		t.Fatalf("load failure = %v, want wrapped command error", err)
	}
	for _, detail := range []string{
		"coding-agent:dev", "da-coding-agent-smoke", "image not present locally",
	} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("load failure omitted %q: %v", detail, err)
		}
	}
}

func TestCommitImageUsesCheckoutRevision(t *testing.T) {
	first, revision, err := CommitImage(
		"declarative-agents/agent-core",
		"0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if first != "declarative-agents/agent-core:0123456789ab" ||
		revision != "0123456789ab" {
		t.Fatalf("commit image = %q revision %q", first, revision)
	}
	second, _, err := CommitImage(
		"declarative-agents/agent-core",
		"fedcba9876543210fedcba9876543210fedcba98")
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatalf("consecutive commits reused image %q", first)
	}
}

func TestCommitImageRejectsMutableOrMissingInputs(t *testing.T) {
	for _, test := range []struct {
		repository string
		revision   string
	}{
		{"", "0123456789abcdef0123456789abcdef01234567"},
		{"declarative-agents/agent-core", ""},
		{"declarative-agents/agent-core", "smoke"},
		{"declarative-agents/agent-core", "0123456789a"},
	} {
		if _, _, err := CommitImage(test.repository, test.revision); err == nil {
			t.Errorf("CommitImage(%q, %q) succeeded", test.repository, test.revision)
		}
	}
}

func TestReleaseAfterFailureCapturesEvidenceBeforeDelete(t *testing.T) {
	var sequence []string
	kindRun := func(args ...string) ([]byte, error) {
		sequence = append(sequence, "kind "+strings.Join(args, " "))
		return nil, nil
	}
	commandRun := func(name string, args ...string) ([]byte, error) {
		sequence = append(sequence, name+" "+strings.Join(args, " "))
		if strings.Contains(strings.Join(args, " "), "get pods") {
			return []byte("pod/planner-0\npod/executor-0\n"), nil
		}
		return []byte("diagnostic"), nil
	}
	dir := filepath.Join(t.TempDir(), "evidence")
	Cluster{Name: "da-coding-agent-smoke", Created: true}.ReleaseAfter(
		kindRun, true, FailureEvidence{
			Directory: dir, Namespaces: []string{"coding-agent-smoke"}, Run: commandRun,
		})

	if len(sequence) != 6 {
		t.Fatalf("sequence = %v, want export, describe, list, two logs, delete", sequence)
	}
	if !strings.HasPrefix(sequence[0], "kind export logs ") ||
		!strings.HasPrefix(sequence[len(sequence)-1], "kind delete cluster ") {
		t.Fatalf("evidence must precede deletion: %v", sequence)
	}
	for _, name := range []string{
		"namespace-coding-agent-smoke-describe.txt",
		"namespace-coding-agent-smoke-pods.txt",
		"namespace-coding-agent-smoke-pod-planner-0-logs.txt",
		"namespace-coding-agent-smoke-pod-executor-0-logs.txt",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("evidence file %s: %v", name, err)
		}
	}
}

func TestReleaseAfterLeavesReusedClusterUntouched(t *testing.T) {
	var calls int
	run := func(_ ...string) ([]byte, error) {
		calls++
		return nil, nil
	}
	commandRun := func(_ string, _ ...string) ([]byte, error) {
		calls++
		return nil, nil
	}
	dir := filepath.Join(t.TempDir(), "must-not-exist")
	Cluster{Name: "developer-cluster"}.ReleaseAfter(run, true, FailureEvidence{
		Directory: dir, Namespaces: []string{"default"}, Run: commandRun,
	})
	if calls != 0 {
		t.Fatalf("reused cluster received %d commands, want none", calls)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("reused cluster created evidence directory: %v", err)
	}
}

func TestReleaseAfterSuccessDeletesWithoutEvidence(t *testing.T) {
	var calls [][]string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return nil, nil
	}
	dir := filepath.Join(t.TempDir(), "must-not-exist")
	Cluster{Name: "owned-cluster", Created: true}.ReleaseAfter(
		run, false, FailureEvidence{Directory: dir})
	if len(calls) != 1 || calls[0][0] != "delete" {
		t.Fatalf("successful release calls = %v, want only delete", calls)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("successful release created evidence directory: %v", err)
	}
}

func TestKubeconfigWritesClusterConfigToPrivateFile(t *testing.T) {
	const content = "apiVersion: v1\nkind: Config\nclusters: []\n"
	kind := &fakeKind{kubeconfig: []byte(content)}
	path, cleanup, err := Kubeconfig(kind.run, "da-chatbot-mesh-smoke")
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}
	defer cleanup()

	call := kind.lastCall("get")
	if len(call) != 4 || call[1] != "kubeconfig" || call[3] != "da-chatbot-mesh-smoke" {
		t.Fatalf("kind get kubeconfig call = %v, want [get kubeconfig --name da-chatbot-mesh-smoke]", call)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read kubeconfig: %v", err)
	}
	if string(got) != content {
		t.Fatalf("kubeconfig content = %q, want %q", got, content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat kubeconfig: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("kubeconfig mode = %o, want 600", perm)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cleanup left kubeconfig behind: %v", err)
	}
}

func TestKubeconfigRejectsEmptyAndErrors(t *testing.T) {
	if _, _, err := Kubeconfig((&fakeKind{kubeconfig: []byte("   \n")}).run, "c"); err == nil {
		t.Fatal("empty kubeconfig must error")
	}
	failing := &fakeKind{kubeconfig: []byte("boom"), kubeconfigErr: errors.New("no such cluster")}
	if _, _, err := Kubeconfig(failing.run, "c"); err == nil {
		t.Fatal("kind get kubeconfig failure must error")
	}
}

func TestBindKubeconfigSetsAndRestores(t *testing.T) {
	t.Run("restores a prior value", func(t *testing.T) {
		t.Setenv("KUBECONFIG", "/ambient/context")
		restore := BindKubeconfig("/cluster/config")
		if got := os.Getenv("KUBECONFIG"); got != "/cluster/config" {
			t.Fatalf("KUBECONFIG = %q, want /cluster/config", got)
		}
		restore()
		if got := os.Getenv("KUBECONFIG"); got != "/ambient/context" {
			t.Fatalf("restored KUBECONFIG = %q, want /ambient/context", got)
		}
	})

	t.Run("unsets when there was no prior value", func(t *testing.T) {
		t.Setenv("KUBECONFIG", "seed") // ensure a known baseline for t.Setenv restore
		_ = os.Unsetenv("KUBECONFIG")
		restore := BindKubeconfig("/cluster/config")
		restore()
		if _, had := os.LookupEnv("KUBECONFIG"); had {
			t.Fatal("KUBECONFIG must be unset when there was no prior value")
		}
	})
}

// TestReusedClusterBindsAwayFromAmbientContext is the isolation regression:
// even when a named cluster already exists (reuse) and the ambient KUBECONFIG
// points at an unrelated cluster, generating and binding the cluster's own
// kubeconfig routes subsequent commands to it rather than the ambient context
// (GH-1341).
func TestReusedClusterBindsAwayFromAmbientContext(t *testing.T) {
	ambient := filepath.Join(t.TempDir(), "ambient-kubeconfig")
	if err := os.WriteFile(ambient, []byte("apiVersion: v1\nkind: Config\n# unrelated cluster\n"), 0o600); err != nil {
		t.Fatalf("write ambient kubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", ambient)

	kind := &fakeKind{
		existing:   []string{"da-chatbot-mesh-smoke"},
		kubeconfig: []byte("apiVersion: v1\nkind: Config\n# da-chatbot-mesh-smoke\n"),
	}
	cluster, err := EnsureClusterWithOptions(
		kind.run, "da-chatbot-mesh-smoke", testConfig(t), 0, healthyEnsureOptions())
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if cluster.Created {
		t.Fatal("pre-existing cluster must be reused, not created")
	}

	path, cleanup, err := Kubeconfig(kind.run, cluster.Name)
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}
	defer cleanup()
	restore := BindKubeconfig(path)
	defer restore()

	if got := os.Getenv("KUBECONFIG"); got != path {
		t.Fatalf("bound KUBECONFIG = %q, want the cluster's %q (not ambient %q)", got, path, ambient)
	}
	if os.Getenv("KUBECONFIG") == ambient {
		t.Fatal("commands would still target the ambient context")
	}
	restore()
	if got := os.Getenv("KUBECONFIG"); got != ambient {
		t.Fatalf("after restore KUBECONFIG = %q, want ambient %q", got, ambient)
	}
}
