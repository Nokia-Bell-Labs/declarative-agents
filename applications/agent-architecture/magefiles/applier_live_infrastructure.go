// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	applierLiveInfraTimeout = 5 * time.Second
	applierLiveDiagTimeout  = 10 * time.Second
)

// applierLiveRunner runs a command with a context deadline, returning combined
// output. smokeEnvironment.run satisfies it; the tests supply their own.
type applierLiveRunner func(context.Context, string, ...string) ([]byte, error)

// applierLiveInfrastructureError distinguishes an unavailable host API or applier-pod
// network/API path from a failure of the applier machine or its Helm assertions, so a
// cluster outage never masquerades as a machine/Helm defect.
type applierLiveInfrastructureError struct {
	Check       string
	Cause       error
	Output      string
	ApplyCause  error
	Diagnostics string
}

func (e *applierLiveInfrastructureError) Error() string {
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

func (e *applierLiveInfrastructureError) Unwrap() error { return e.Cause }

// applierLiveSemanticError marks a failure reached with both API paths healthy. The
// semantic assertion remains the cause; diagnostics add evidence without changing
// what constitutes success.
type applierLiveSemanticError struct {
	Step        string
	Cause       error
	Diagnostics string
}

func (e *applierLiveSemanticError) Error() string {
	message := fmt.Sprintf("applierLive %s semantic failure: %v", e.Step, e.Cause)
	if e.Diagnostics != "" {
		message += "\n" + e.Diagnostics
	}
	return message
}

func (e *applierLiveSemanticError) Unwrap() error { return e.Cause }

type applierLiveProbe struct {
	check string
	name  string
	args  []string
}

func applierLiveInfrastructureProbes() []applierLiveProbe {
	requestTimeout := applierLiveInfraTimeout.String()
	deployment := "deployment/" + smokeRelease + "-agent-architecture-applier"
	return []applierLiveProbe{
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
			// kubectl must NOT carry it: --request-timeout routes kubectl's config load
			// through the explicit-flag path, which skips in-cluster detection and
			// falls back to http://localhost:8080, so a perfectly wired pod reports a
			// connection-refused that reads like an SA/token gap (GH-1175). The
			// applier's own exec words never pass --request-timeout, so dropping it here
			// matches how the applier actually reaches the API.
			//
			// -c applier targets the applier container explicitly: the chart
			// delivers itself as a volume (GH-1369), so the pod also has a
			// stage-chart init container, and a bare `kubectl exec` prints a
			// "Defaulted container ... out of: applier, stage-chart (init)"
			// notice that would corrupt the readyz output the probe compares
			// against "ok" (GH-1403).
			args: []string{
				"--request-timeout=" + requestTimeout,
				"-n", smokeNamespace,
				"exec", deployment, "-c", "applier", "--",
				"kubectl", "get", "--raw=/readyz",
			},
		},
	}
}

// checkApplierLiveInfrastructure proves the host API path and then runs kubectl inside
// the real applier pod. The nested kubectl uses the pod's mounted service-account
// token and cluster network, exactly like the declared exec words that perform the
// upgrade and rollout reads.
func checkApplierLiveInfrastructure(run applierLiveRunner) error {
	for _, probe := range applierLiveInfrastructureProbes() {
		ctx, cancel := context.WithTimeout(context.Background(), applierLiveInfraTimeout)
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
		return &applierLiveInfrastructureError{
			Check:  probe.check,
			Cause:  err,
			Output: string(out),
		}
	}
	return nil
}

type applierLiveDiagnostic struct {
	label string
	name  string
	args  []string
}

func applierLiveDiagnosticCommands() []applierLiveDiagnostic {
	return []applierLiveDiagnostic{
		{"helm status", "helm", []string{"status", smokeRelease, "-n", smokeNamespace}},
		{"helm history", "helm", []string{"history", smokeRelease, "-n", smokeNamespace}},
		{"pod readiness", "kubectl", []string{"get", "pods", "-n", smokeNamespace, "-o", "wide"}},
		{"events", "kubectl", []string{"get", "events", "-n", smokeNamespace, "--sort-by=.metadata.creationTimestamp"}},
		{"applier logs", "kubectl", []string{
			"logs", "-n", smokeNamespace, "-l", "app.kubernetes.io/component=applier", "--tail=60",
		}},
	}
}

// collectApplierLiveDiagnostics gives the whole bundle one deadline rather than
// multiplying a timeout by the number of commands. Failures are evidence too: every
// section is emitted, and commands reached after the budget expires see an
// already-cancelled context and return immediately.
func collectApplierLiveDiagnostics(run applierLiveRunner, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var report strings.Builder
	report.WriteString("applierLive bounded diagnostics:")
	for _, diagnostic := range applierLiveDiagnosticCommands() {
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

// runApplierLiveApplyStep preflights both API paths, runs the semantic operation, and
// rechecks infrastructure only when that operation fails. A healthy recheck makes the
// original failure semantic; an unhealthy recheck prevents a cluster outage from
// masquerading as a machine/Helm defect.
func runApplierLiveApplyStep(run applierLiveRunner, step string, operation func() error) error {
	if err := checkApplierLiveInfrastructure(run); err != nil {
		return err
	}
	if err := operation(); err != nil {
		infrastructureErr := checkApplierLiveInfrastructure(run)
		diagnostics := collectApplierLiveDiagnostics(run, applierLiveDiagTimeout)
		var unavailable *applierLiveInfrastructureError
		if errors.As(infrastructureErr, &unavailable) {
			unavailable.ApplyCause = err
			unavailable.Diagnostics = diagnostics
			return unavailable
		}
		return &applierLiveSemanticError{
			Step:        step,
			Cause:       err,
			Diagnostics: diagnostics,
		}
	}
	return nil
}
