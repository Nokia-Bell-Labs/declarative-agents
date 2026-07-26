// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
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
	existing  []string
	calls     [][]string
	createErr error
	deleteErr error
	listErr   error
	exportErr error
}

func (f *fakeKind) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case len(args) >= 2 && args[0] == "get" && args[1] == "clusters":
		if f.listErr != nil {
			return nil, f.listErr
		}
		return []byte(strings.Join(f.existing, "\n") + "\n"), nil
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
	kind := &fakeKind{existing: []string{"da-chatbot-mesh-smoke"}}
	cluster, err := EnsureCluster(kind.run, "da-chatbot-mesh-smoke", testConfig(t), 120*time.Second)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if cluster.Created {
		t.Error("a pre-existing cluster must never be owned by this run")
	}
	if kind.issued("create") {
		t.Error("must not re-create an existing cluster")
	}
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
