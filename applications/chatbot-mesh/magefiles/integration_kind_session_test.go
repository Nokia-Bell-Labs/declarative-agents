// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

func TestIntegrationKindSessionOwnsOneAggregateClusterLifecycle(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	var calls []string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()

	if !adoptAggregateKindCluster(
		kindrig.Cluster{Name: aggregateKindCluster, Created: true},
		run,
		kindrig.FailureEvidence{Directory: t.TempDir()},
	) {
		t.Fatal("active aggregate did not adopt its created cluster")
	}
	if !adoptAggregateKindCluster(
		kindrig.Cluster{Name: aggregateKindCluster},
		run,
		kindrig.FailureEvidence{},
	) {
		t.Fatal("same aggregate cluster was not recognized")
	}
	session.close()

	want := []string{"delete cluster --name " + aggregateKindCluster}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("kind calls = %v, want %v", calls, want)
	}
}

func TestIntegrationKindSessionLeavesDeveloperClusterUntouched(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	called := false
	run := func(...string) ([]byte, error) {
		called = true
		return nil, nil
	}
	session.cluster = kindrig.Cluster{Name: aggregateKindCluster}
	session.kindRun = run
	session.close()
	if called {
		t.Fatal("session mutated a developer-owned cluster")
	}
}

func TestIntegrationKindSessionCapturesEvidenceBeforeOwnedDelete(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	var calls []string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	evidenceDir := filepath.Join(t.TempDir(), "evidence")
	wantErr := errors.New("scenario failed")
	err = session.runTarget("helmSmoke", func() error {
		if !adoptAggregateKindCluster(
			kindrig.Cluster{Name: aggregateKindCluster, Created: true},
			run,
			kindrig.FailureEvidence{Directory: evidenceDir},
		) {
			t.Fatal("cluster adoption failed")
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("target error = %v, want %v", err, wantErr)
	}
	if len(calls) != 2 ||
		!strings.HasPrefix(calls[0], "export logs ") ||
		calls[1] != "delete cluster --name "+aggregateKindCluster {
		t.Fatalf("evidence/delete ordering = %v", calls)
	}
}

func TestIntegrationKindSessionPoisonBlocksLaterSharedTargets(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	wantErr := errors.New("contaminated")
	if err := session.runTarget("helmSmoke", func() error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("first error = %v, want %v", err, wantErr)
	}
	called := false
	err := session.runTarget("helmSwap", func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned") {
		t.Fatalf("later target error = %v, want poisoned session", err)
	}
	if called {
		t.Fatal("later shared target ran after session poison")
	}
}

func TestIntegrationTargetRosterKeepsPolicyProofOutsideSharedSession(t *testing.T) {
	var shared []string
	for _, target := range integrationTargets(Integration{}) {
		if target.sharedKind {
			shared = append(shared, target.name)
		}
		if target.name == "policyProof" && target.sharedKind {
			t.Fatal("policy-proof entered the default-CNI shared session")
		}
	}
	want := []string{"helmSmoke", "helmSwap", "helmLLMTier", "applierLive"}
	if !reflect.DeepEqual(shared, want) {
		t.Fatalf("shared targets = %v, want %v", shared, want)
	}
}

func TestDirectIntegrationTargetHasNoSharedSession(t *testing.T) {
	if activeIntegrationKindSession() != nil {
		t.Fatal("test started with leaked aggregate session")
	}
	if adoptAggregateKindCluster(
		kindrig.Cluster{Name: aggregateKindCluster, Created: true},
		kindrig.DefaultRun,
		kindrig.FailureEvidence{},
	) {
		t.Fatal("direct target transferred ownership without an aggregate")
	}
}
