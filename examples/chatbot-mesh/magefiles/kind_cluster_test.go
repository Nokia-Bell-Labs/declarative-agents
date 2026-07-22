// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"strings"
	"testing"
)

// fakeKind records the kind subcommands a run issues and replays a scripted
// cluster list, so ownership is proven without creating a real cluster.
type fakeKind struct {
	existing  []string
	calls     [][]string
	createErr error
	deleteErr error
	listErr   error
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

// TestKindEnsureClusterCreatesWhenAbsent covers the absent case: no cluster
// exists, so the run creates one and owns it.
func TestKindEnsureClusterCreatesWhenAbsent(t *testing.T) {
	kind := &fakeKind{existing: []string{"some-other-cluster"}}
	cluster, err := kindEnsureCluster(kind.run, "chatbot-mesh-smoke")
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

// TestKindEnsureClusterReusesPreExistingWithoutOwnership covers the pre-existing
// case: the run reuses the cluster and must not claim ownership.
func TestKindEnsureClusterReusesPreExistingWithoutOwnership(t *testing.T) {
	kind := &fakeKind{existing: []string{"chatbot-mesh-smoke"}}
	cluster, err := kindEnsureCluster(kind.run, "chatbot-mesh-smoke")
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

// TestKindClusterReleaseDeletesOnlyWhatThisRunCreated is the regression guard for
// GH-589: releasing a reused cluster must issue no delete, so an integration run
// cannot destroy a developer or CI cluster it did not create.
func TestKindClusterReleaseDeletesOnlyWhatThisRunCreated(t *testing.T) {
	tests := []struct {
		name       string
		cluster    kindCluster
		wantDelete bool
	}{
		{"created by this run", kindCluster{Name: "chatbot-mesh-smoke", Created: true}, true},
		{"pre-existing", kindCluster{Name: "chatbot-mesh-smoke"}, false},
		{"never acquired", kindCluster{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind := &fakeKind{}
			tt.cluster.release(kind.run)
			if got := kind.issued("delete"); got != tt.wantDelete {
				t.Errorf("delete issued = %v, want %v", got, tt.wantDelete)
			}
		})
	}
}

// TestKindEnsureClusterReportsCreateFailure covers the create-failed case: the
// error propagates and nothing is claimed as owned, so no delete follows.
func TestKindEnsureClusterReportsCreateFailure(t *testing.T) {
	kind := &fakeKind{createErr: fmt.Errorf("exit status 1")}
	cluster, err := kindEnsureCluster(kind.run, "chatbot-mesh-smoke")
	if err == nil {
		t.Fatal("a failed create must be reported")
	}
	if cluster.Created {
		t.Error("a failed create must not be owned")
	}
	cluster.release(kind.run)
	if kind.issued("delete") {
		t.Error("a cluster that was never created must not be deleted")
	}
}

// TestKindClusterReleaseToleratesCleanupFailure covers the cleanup-failed case: a
// delete error is reported but does not panic or block the caller's own result.
func TestKindClusterReleaseToleratesCleanupFailure(t *testing.T) {
	kind := &fakeKind{deleteErr: fmt.Errorf("exit status 1")}
	kindCluster{Name: "chatbot-mesh-smoke", Created: true}.release(kind.run)
	if !kind.issued("delete") {
		t.Error("an owned cluster must still attempt deletion")
	}
}

// TestKindClusterExistsTreatsListFailureAsAbsent asserts an unreadable cluster
// list does not report a cluster as pre-existing. Ensure then attempts a create,
// whose own error surfaces, rather than silently reusing an unknown cluster.
func TestKindClusterExistsTreatsListFailureAsAbsent(t *testing.T) {
	kind := &fakeKind{listErr: fmt.Errorf("kind not on PATH")}
	if kindClusterExists(kind.run, "chatbot-mesh-smoke") {
		t.Error("an unreadable cluster list must not report the cluster as present")
	}
}
