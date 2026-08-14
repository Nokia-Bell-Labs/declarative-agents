// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

func TestCodingApplierLiveOwnsDedicatedClusterRecovery(t *testing.T) {
	options := codingApplierLiveEnsureOptions()
	if options.ReusePolicy != kindrig.RecreateUnhealthyOwnedCluster {
		t.Fatalf("reuse policy = %v, want explicit owned-cluster recovery", options.ReusePolicy)
	}
	if options.HealthRun == nil {
		t.Fatal("dedicated-cluster recovery must provide a bounded health runner")
	}
}

func TestCodingApplierLiveRollbackHookKeepsRollbackInsideRequestBudget(t *testing.T) {
	for _, want := range []string{
		`"progressDeadlineSeconds":5`,
		`"image":"invalid.local/applier-live-rollback:missing"`,
	} {
		if !strings.Contains(codingApplierLiveRollbackHook, want) {
			t.Fatalf("rollback hook missing %q", want)
		}
	}
}

func TestCodingApplierLiveInfrastructureHealthy(t *testing.T) {
	var calls []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return []byte("ok\n"), nil
	}

	if err := checkCodingApplierLiveInfrastructure(run); err != nil {
		t.Fatalf("healthy infrastructure rejected: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want host and pod probes", calls)
	}
	if !strings.Contains(calls[0], "get --raw=/readyz") {
		t.Errorf("host probe = %q, want readyz", calls[0])
	}
	for _, want := range []string{
		"exec deployment/" + codingHelmRelease + "-coding-agent-applier",
		"-n " + codingHelmNamespace,
		// -c applier targets the applier container so the stage-chart init
		// container (GH-1369) does not trigger a "Defaulted container" notice
		// that corrupts the readyz comparison (GH-1403).
		"-c applier",
		"-- kubectl",
		"get --raw=/readyz",
	} {
		if !strings.Contains(calls[1], want) {
			t.Errorf("pod probe = %q, want %q", calls[1], want)
		}
	}
}

func TestCodingApplierLiveInfrastructureClassifiesHostAPIFailure(t *testing.T) {
	calls := 0
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		return []byte("tls handshake timeout"), errors.New("host API unavailable")
	}

	err := checkCodingApplierLiveInfrastructure(run)
	var unavailable *codingApplierLiveInfrastructureError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %T %v, want infrastructure classification", err, err)
	}
	if unavailable.Check != "host-to-Kubernetes API" {
		t.Errorf("check = %q", unavailable.Check)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want pod probe skipped after host failure", calls)
	}
}

func TestCodingApplierLiveInfrastructureClassifiesPodAPIFailure(t *testing.T) {
	calls := 0
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("ok"), nil
		}
		return []byte("dial tcp 10.96.0.1:443: i/o timeout"), errors.New("pod network unavailable")
	}

	err := checkCodingApplierLiveInfrastructure(run)
	var unavailable *codingApplierLiveInfrastructureError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %T %v, want infrastructure classification", err, err)
	}
	if !strings.Contains(unavailable.Check, "service account") {
		t.Errorf("check = %q, want pod service-account path", unavailable.Check)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want both probes", calls)
	}
}

func TestCodingApplierLiveDiagnosticsAreBoundedAndKeepFailures(t *testing.T) {
	t.Run("overall timeout", func(t *testing.T) {
		started := time.Now()
		calls := 0
		run := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			<-ctx.Done()
			return nil, ctx.Err()
		}

		report := collectCodingApplierLiveDiagnostics(run, 5*time.Millisecond)
		if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
			t.Fatalf("diagnostics took %s, want bounded completion", elapsed)
		}
		if calls != len(codingApplierLiveDiagnosticCommands()) {
			t.Errorf("calls = %d, want all %d diagnostic sections", calls, len(codingApplierLiveDiagnosticCommands()))
		}
		for _, want := range []string{"helm status", "helm history", "pod readiness", "events", "applier logs", "deadline exceeded"} {
			if !strings.Contains(report, want) {
				t.Errorf("report missing %q:\n%s", want, report)
			}
		}
	})

	t.Run("command failures", func(t *testing.T) {
		run := func(_ context.Context, name string, args ...string) ([]byte, error) {
			command := name + " " + strings.Join(args, " ")
			return []byte("partial output for " + command), fmt.Errorf("%s failed", name)
		}

		report := collectCodingApplierLiveDiagnostics(run, time.Second)
		if count := strings.Count(report, "[diagnostic failed:"); count != len(codingApplierLiveDiagnosticCommands()) {
			t.Errorf("failure count = %d, want %d:\n%s", count, len(codingApplierLiveDiagnosticCommands()), report)
		}
		if !strings.Contains(report, "partial output") {
			t.Errorf("report discarded command output:\n%s", report)
		}
	})
}

func TestCodingApplierLiveApplyFailureClassification(t *testing.T) {
	t.Run("healthy infrastructure preserves semantic failure", func(t *testing.T) {
		run := func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "kubectl" {
				return []byte("ok"), nil
			}
			return []byte("diagnostic"), nil
		}
		cause := errors.New("revision did not advance")
		err := runApplierLiveApplyStep(run, "upgrade", func() error { return cause })

		var semantic *codingApplierLiveSemanticError
		if !errors.As(err, &semantic) {
			t.Fatalf("error = %T %v, want semantic classification", err, err)
		}
		if !errors.Is(err, cause) {
			t.Errorf("semantic error does not preserve cause: %v", err)
		}
		if !strings.Contains(err.Error(), "bounded diagnostics") {
			t.Errorf("semantic failure missing diagnostics: %v", err)
		}
	})

	t.Run("failed recheck classifies infrastructure", func(t *testing.T) {
		probes := 0
		run := func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "kubectl" || !strings.Contains(strings.Join(args, " "), "/readyz") {
				return []byte("diagnostic"), nil
			}
			probes++
			if probes <= 2 {
				return []byte("ok"), nil
			}
			return []byte("TLS handshake timeout"), errors.New("API unavailable")
		}
		cause := errors.New("machine timeout")
		err := runApplierLiveApplyStep(run, "upgrade", func() error { return cause })

		var unavailable *codingApplierLiveInfrastructureError
		if !errors.As(err, &unavailable) {
			t.Fatalf("error = %T %v, want infrastructure classification", err, err)
		}
		if !strings.Contains(err.Error(), cause.Error()) {
			t.Errorf("infrastructure failure omitted original apply failure: %v", err)
		}
		if !strings.Contains(err.Error(), "bounded diagnostics") {
			t.Errorf("infrastructure failure missing diagnostics: %v", err)
		}
	})
}

func TestCodingApplierLiveSkipReasonHidesMissingBinary(t *testing.T) {
	t.Setenv("PATH", "")
	reason := applierLiveSkipReason(integrationRoots{}, func(
		context.Context, string, ...string,
	) ([]byte, error) {
		t.Fatal("runner must not execute after a missing binary")
		return nil, nil
	})
	if !strings.Contains(reason, "docker not found") {
		t.Fatalf("skip reason = %q, want docker not found", reason)
	}
}
