// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestApplierLiveUsesSharedEnsurePath(t *testing.T) {
	body, err := os.ReadFile("integration_applier_live.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	if strings.Contains(source, "docker\", \"build\"") || strings.Contains(source, "docker build") {
		t.Fatal("unlabeled docker build would retag declarative-agents/applier:<rev> without identity labels")
	}
	if !strings.Contains(source, "kindrig.EnsureApplierImage") {
		t.Fatal("live applier must use the shared ensure path so concurrent lanes serialize on the tag")
	}
}

func TestApplierLiveInfrastructureHealthy(t *testing.T) {
	var calls []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return []byte("ok\n"), nil
	}

	if err := checkApplierLiveInfrastructure(run); err != nil {
		t.Fatalf("healthy infrastructure rejected: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want host and pod probes", calls)
	}
	if !strings.Contains(calls[0], "get --raw=/readyz") {
		t.Errorf("host probe = %q, want readyz", calls[0])
	}
	for _, want := range []string{
		"exec deployment/" + smokeRelease + "-agent-architecture-applier",
		"-n " + smokeNamespace,
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
	// The in-pod kubectl must not carry --request-timeout (GH-1175): only the outer
	// host kubectl does.
	inner := calls[1]
	if idx := strings.Index(inner, "-- kubectl"); idx >= 0 {
		if strings.Contains(inner[idx:], "--request-timeout") {
			t.Errorf("in-pod probe carries --request-timeout, which breaks in-cluster config: %q", inner)
		}
	}
}

func TestApplierLiveInfrastructureClassifiesHostAPIFailure(t *testing.T) {
	calls := 0
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		return []byte("tls handshake timeout"), errors.New("host API unavailable")
	}

	err := checkApplierLiveInfrastructure(run)
	var unavailable *applierLiveInfrastructureError
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

func TestApplierLiveInfrastructureClassifiesPodAPIFailure(t *testing.T) {
	calls := 0
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("ok"), nil
		}
		return []byte("dial tcp 10.96.0.1:443: i/o timeout"), errors.New("pod network unavailable")
	}

	err := checkApplierLiveInfrastructure(run)
	var unavailable *applierLiveInfrastructureError
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

func TestApplierLiveDiagnosticsAreBoundedAndKeepFailures(t *testing.T) {
	t.Run("overall timeout", func(t *testing.T) {
		started := time.Now()
		calls := 0
		run := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			<-ctx.Done()
			return nil, ctx.Err()
		}

		report := collectApplierLiveDiagnostics(run, 5*time.Millisecond)
		if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
			t.Fatalf("diagnostics took %s, want bounded completion", elapsed)
		}
		if calls != len(applierLiveDiagnosticCommands()) {
			t.Errorf("calls = %d, want all %d diagnostic sections", calls, len(applierLiveDiagnosticCommands()))
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

		report := collectApplierLiveDiagnostics(run, time.Second)
		if count := strings.Count(report, "[diagnostic failed:"); count != len(applierLiveDiagnosticCommands()) {
			t.Errorf("failure count = %d, want %d:\n%s", count, len(applierLiveDiagnosticCommands()), report)
		}
		if !strings.Contains(report, "partial output") {
			t.Errorf("report discarded command output:\n%s", report)
		}
	})
}

func TestApplierLiveApplyFailureClassification(t *testing.T) {
	t.Run("healthy infrastructure preserves semantic failure", func(t *testing.T) {
		run := func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "kubectl" {
				return []byte("ok"), nil
			}
			return []byte("diagnostic"), nil
		}
		cause := errors.New("revision did not advance")
		err := runApplierLiveApplyStep(run, "upgrade", func() error { return cause })

		var semantic *applierLiveSemanticError
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

		var unavailable *applierLiveInfrastructureError
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

func TestApplierLiveSkipReasonHidesMissingBinary(t *testing.T) {
	t.Setenv("PATH", "")
	reason := smokeSkipReason(roots{})
	if !strings.Contains(reason, "docker not found") {
		t.Fatalf("skip reason = %q, want docker not found", reason)
	}
}
