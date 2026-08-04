// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

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
		"exec deployment/live-chatbot-mesh-applier",
		// -c applier targets the applier container so the stage-chart init
		// container (GH-1368) does not trigger a "Defaulted container" notice
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
	// host kubectl does. The flag routes kubectl through the explicit-flag config
	// path, which skips in-cluster detection and falls back to localhost:8080.
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

func TestApplierLiveInitialReadinessFailureCollectsDiagnostics(t *testing.T) {
	var sequence []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		sequence = append(sequence, command)
		return []byte("evidence from " + command), nil
	}
	cause := errors.New("deployment exceeded its progress deadline")
	err := waitApplierLiveInitialReadiness(run, func() error {
		sequence = append(sequence, "wait for initial readiness")
		return cause
	})

	var semantic *applierLiveSemanticError
	if !errors.As(err, &semantic) {
		t.Fatalf("error = %T %v, want semantic readiness failure", err, err)
	}
	if semantic.Step != "initial readiness" || !errors.Is(err, cause) {
		t.Fatalf("readiness error did not preserve step and cause: %v", err)
	}
	if len(sequence) != len(applierLiveDiagnosticCommands())+1 {
		t.Fatalf("sequence has %d steps, want wait plus %d diagnostics:\n%s",
			len(sequence), len(applierLiveDiagnosticCommands()), strings.Join(sequence, "\n"))
	}
	if sequence[0] != "wait for initial readiness" {
		t.Fatalf("diagnostics ran before readiness failed:\n%s", strings.Join(sequence, "\n"))
	}
	for _, want := range []string{
		"actual release Secret sizes",
		"Deployment ReplicaSet and pod status",
		"stage-chart init logs",
		"previous applier logs",
		"Service and endpoints",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("initial-readiness error missing %q:\n%v", want, err)
		}
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
		for _, want := range []string{
			"Helm status",
			"Helm history",
			"actual release Secret sizes",
			"applier Deployment ReplicaSet and pod status",
			"container and init status",
			"stage-chart init logs",
			"previous applier logs",
			"node capacity",
			"chart ConfigMap archive key",
			"applier Service and endpoints",
			"deadline exceeded",
		} {
			if !strings.Contains(report, want) {
				t.Errorf("report missing %q:\n%s", want, report)
			}
		}
	})

	t.Run("partial command failures", func(t *testing.T) {
		calls := 0
		run := func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls++
			command := name + " " + strings.Join(args, " ")
			if calls%3 == 0 {
				return []byte("partial output for " + command), fmt.Errorf("%s failed", name)
			}
			return []byte("complete output for " + command), nil
		}

		report := collectApplierLiveDiagnostics(run, time.Second)
		if calls != len(applierLiveDiagnosticCommands()) {
			t.Errorf("calls = %d, want every one of %d diagnostics", calls, len(applierLiveDiagnosticCommands()))
		}
		if count := strings.Count(report, "[diagnostic failed:"); count != calls/3 {
			t.Errorf("failure count = %d, want %d:\n%s", count, calls/3, report)
		}
		for _, want := range []string{"partial output", "complete output", "applier Service and endpoints"} {
			if !strings.Contains(report, want) {
				t.Errorf("report discarded %q:\n%s", want, report)
			}
		}
	})
}

func TestApplierLiveDiagnosticsIncludeInitPreviousAndOrderedBootstrapEvidence(t *testing.T) {
	commands := applierLiveDiagnosticCommands()
	var labels, invocations []string
	for _, command := range commands {
		labels = append(labels, command.label)
		invocations = append(invocations, command.name+" "+strings.Join(command.args, " "))
	}
	wantLabels := []string{
		"Helm status",
		"Helm history",
		"actual release Secret sizes",
		"applier Deployment ReplicaSet and pod status",
		"container and init status",
		"applier pod scheduling probes and termination",
		"events",
		"stage-chart init logs",
		"current applier logs",
		"previous applier logs",
		"node capacity and allocatable resources",
		"chart ConfigMap archive key",
		"applier Service and endpoints",
	}
	if strings.Join(labels, "\n") != strings.Join(wantLabels, "\n") {
		t.Fatalf("diagnostic order:\n got: %v\nwant: %v", labels, wantLabels)
	}
	joined := strings.Join(invocations, "\n")
	for _, want := range []string{
		"get deployment,replicaset,pod",
		`go-template={{range .items}}`,
		"base64decode | len",
		"describe pods",
		"-o json",
		"logs -l " + "app.kubernetes.io/instance=" + applierLiveRelease +
			",app.kubernetes.io/component=applier -c stage-chart",
		"-c applier --tail=120",
		"-c applier --previous --tail=120",
		"CAPACITY_MEMORY:.status.capacity.memory",
		"configmap " + applierLiveChartConfigMap,
		"service/live-chatbot-mesh-applier endpoints/live-chatbot-mesh-applier",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("diagnostic commands missing %q:\n%s", want, joined)
		}
	}
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
