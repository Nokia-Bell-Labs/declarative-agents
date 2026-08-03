// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	codingApplierLiveInfraTimeout = 5 * time.Second
	codingApplierLiveDiagTimeout  = 10 * time.Second
)

// codingApplierLiveInfrastructureError distinguishes an unavailable host API or
// applier-pod network/API path from a failure of the applier machine or its Helm
// assertions, so a cluster outage never masquerades as a machine/Helm defect.
type codingApplierLiveInfrastructureError struct {
	Check       string
	Cause       error
	Output      string
	ApplyCause  error
	Diagnostics string
}

func (e *codingApplierLiveInfrastructureError) Error() string {
	message := fmt.Sprintf("applierLive infrastructure unavailable at %s: %v", e.Check, e.Cause)
	if output := strings.TrimSpace(e.Output); output != "" {
		message += "\nprobe output:\n" + output
	}
	if e.ApplyCause != nil {
		message += "\napply failure observed before infrastructure recheck: " + e.ApplyCause.Error()
	}
	if e.Diagnostics != "" {
		message += "\n" + e.Diagnostics
	}
	return message
}

func (e *codingApplierLiveInfrastructureError) Unwrap() error { return e.Cause }

// codingApplierLiveSemanticError marks a failure reached with both API paths
// healthy. The semantic assertion remains the cause; diagnostics add evidence
// without changing what constitutes success.
type codingApplierLiveSemanticError struct {
	Step        string
	Cause       error
	Diagnostics string
}

func (e *codingApplierLiveSemanticError) Error() string {
	message := fmt.Sprintf("applierLive %s semantic failure: %v", e.Step, e.Cause)
	if e.Diagnostics != "" {
		message += "\n" + e.Diagnostics
	}
	return message
}

func (e *codingApplierLiveSemanticError) Unwrap() error { return e.Cause }

type codingApplierLiveProbe struct {
	check string
	name  string
	args  []string
}

func codingApplierLiveInfrastructureProbes() []codingApplierLiveProbe {
	requestTimeout := codingApplierLiveInfraTimeout.String()
	deployment := "deployment/" + codingHelmRelease + "-coding-agent-applier"
	return []codingApplierLiveProbe{
		{
			check: "host-to-Kubernetes API",
			name:  "kubectl",
			args:  []string{"--request-timeout=" + requestTimeout, "get", "--raw=/readyz"},
		},
		{
			check: "applier-pod-to-Kubernetes API using its service account",
			name:  "kubectl",
			// The outer (host) kubectl keeps --request-timeout: it loads an explicit
			// kubeconfig, so the flag only bounds the exec RPC. The inner (in-pod)
			// kubectl must NOT carry it: --request-timeout routes kubectl's config
			// load through the explicit-flag path, which skips in-cluster detection
			// and falls back to http://localhost:8080, so a perfectly wired pod
			// reports a connection-refused that reads like an SA/token gap. The
			// applier's own exec words never pass --request-timeout (they use the
			// rollout-status --timeout and -o go-template), so dropping it here
			// matches how the applier actually reaches the API. The 5s context in
			// checkCodingApplierLiveInfrastructure and the outer --request-timeout
			// keep the whole probe bounded.
			//
			// -c applier targets the applier container explicitly: the chart
			// delivers itself as a volume (GH-1369), so the pod also has a
			// stage-chart init container, and a bare `kubectl exec` prints a
			// "Defaulted container ... out of: applier, stage-chart (init)"
			// notice that would corrupt the readyz output compared against "ok"
			// (GH-1403).
			args: []string{
				"--request-timeout=" + requestTimeout,
				"-n", codingHelmNamespace,
				"exec", deployment, "-c", "applier", "--",
				"kubectl", "get", "--raw=/readyz",
			},
		},
	}
}

// checkCodingApplierLiveInfrastructure proves the host API path and then runs
// kubectl inside the real applier pod. The nested kubectl uses the pod's mounted
// service-account token and cluster network, exactly like the declared exec words
// that perform the upgrade and rollout reads.
func checkCodingApplierLiveInfrastructure(run codingSmokeRunner) error {
	for _, probe := range codingApplierLiveInfrastructureProbes() {
		ctx, cancel := context.WithTimeout(context.Background(), codingApplierLiveInfraTimeout)
		out, err := run(ctx, probe.name, probe.args...)
		contextErr := ctx.Err()
		cancel()
		if err == nil && contextErr == nil && strings.TrimSpace(string(out)) == "ok" {
			continue
		}
		if contextErr != nil {
			err = contextErr
		} else if err == nil {
			err = fmt.Errorf("readyz returned %q, want %q", strings.TrimSpace(string(out)), "ok")
		}
		return &codingApplierLiveInfrastructureError{
			Check:  probe.check,
			Cause:  err,
			Output: string(out),
		}
	}
	return nil
}

type codingApplierLiveDiagnostic struct {
	label string
	name  string
	args  []string
}

func codingApplierLiveDiagnosticCommands() []codingApplierLiveDiagnostic {
	return []codingApplierLiveDiagnostic{
		{"helm status", "helm", []string{"status", codingHelmRelease, "-n", codingHelmNamespace}},
		{"helm history", "helm", []string{"history", codingHelmRelease, "-n", codingHelmNamespace}},
		{"pod readiness", "kubectl", []string{"get", "pods", "-n", codingHelmNamespace, "-o", "wide"}},
		{"events", "kubectl", []string{"get", "events", "-n", codingHelmNamespace, "--sort-by=.metadata.creationTimestamp"}},
		{"applier logs", "kubectl", []string{
			"logs", "-n", codingHelmNamespace, "-l", "app.kubernetes.io/component=applier", "--tail=60",
		}},
	}
}

// collectCodingApplierLiveDiagnostics gives the whole bundle one deadline rather
// than multiplying a timeout by the number of commands. Failures are evidence
// too: every section is emitted, and commands reached after the budget expires
// see an already-cancelled context and return immediately.
func collectCodingApplierLiveDiagnostics(run codingSmokeRunner, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var report strings.Builder
	report.WriteString("applierLive bounded diagnostics:")
	for _, diagnostic := range codingApplierLiveDiagnosticCommands() {
		out, err := run(ctx, diagnostic.name, diagnostic.args...)
		report.WriteString("\n\n== " + diagnostic.label + " ==")
		if text := strings.TrimSpace(string(out)); text != "" {
			report.WriteString("\n" + text)
		}
		if err != nil {
			report.WriteString("\n[diagnostic failed: " + err.Error() + "]")
		} else if ctx.Err() != nil {
			report.WriteString("\n[diagnostic failed: " + ctx.Err().Error() + "]")
		}
	}
	return report.String()
}

// runApplierLiveApplyStep preflights both API paths, runs the semantic operation,
// and rechecks infrastructure only when that operation fails. A healthy recheck
// makes the original failure semantic; an unhealthy recheck prevents a cluster
// outage from masquerading as a machine/Helm defect.
func runApplierLiveApplyStep(run codingSmokeRunner, step string, operation func() error) error {
	if err := checkCodingApplierLiveInfrastructure(run); err != nil {
		return err
	}
	if err := operation(); err != nil {
		infrastructureErr := checkCodingApplierLiveInfrastructure(run)
		diagnostics := collectCodingApplierLiveDiagnostics(run, codingApplierLiveDiagTimeout)
		var unavailable *codingApplierLiveInfrastructureError
		if errors.As(infrastructureErr, &unavailable) {
			unavailable.ApplyCause = err
			unavailable.Diagnostics = diagnostics
			return unavailable
		}
		return &codingApplierLiveSemanticError{
			Step:        step,
			Cause:       err,
			Diagnostics: diagnostics,
		}
	}
	return nil
}
