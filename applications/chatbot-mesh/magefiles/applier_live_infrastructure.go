// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	applierLiveInfrastructureTimeout = 5 * time.Second
	applierLiveDiagnosticsTimeout    = 10 * time.Second
)

// applierLiveCommandRunner is the context-aware command boundary for live
// infrastructure probes and diagnostics. Tests supply deterministic runners;
// production uses exec.CommandContext so every probe remains bounded even when
// Docker Desktop or the Kubernetes API has stopped answering.
type applierLiveCommandRunner func(context.Context, string, ...string) ([]byte, error)

func runApplierLiveCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// applierLiveInfrastructureError distinguishes an unavailable host API or pod
// network/API path from a failure of the applier machine or its Helm assertions.
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

// applierLiveSemanticError marks a failure reached with both API paths healthy.
// The existing semantic assertion remains the cause; diagnostics add evidence
// without changing what constitutes success.
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

type applierLiveInfrastructureProbe struct {
	check string
	name  string
	args  []string
}

func applierLiveInfrastructureProbes() []applierLiveInfrastructureProbe {
	requestTimeout := applierLiveInfrastructureTimeout.String()
	deployment := "deployment/" + applierLiveRelease + "-chatbot-mesh-applier"
	return []applierLiveInfrastructureProbe{
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
			// reports a connection-refused that reads like an SA/token gap (GH-1175).
			// The applier's own exec words never pass --request-timeout, so dropping
			// it here matches how the applier actually reaches the API.
			//
			// -c applier targets the applier container explicitly: the chart is
			// delivered as a volume (GH-1368), so the pod also has a stage-chart
			// init container, and a bare `kubectl exec` prints a "Defaulted
			// container ... out of: applier, stage-chart (init)" notice that
			// corrupts the readyz comparison (GH-1403).
			args: []string{
				"--request-timeout=" + requestTimeout,
				"exec", deployment, "-c", "applier", "--",
				"kubectl", "get", "--raw=/readyz",
			},
		},
	}
}

// checkApplierLiveInfrastructure proves the host API path and then runs kubectl
// inside the real applier pod. The nested kubectl uses the pod's mounted
// service-account token and cluster network, exactly like the declared exec
// words that will perform the upgrade and rollout reads.
func checkApplierLiveInfrastructure(run applierLiveCommandRunner) error {
	for _, probe := range applierLiveInfrastructureProbes() {
		ctx, cancel := context.WithTimeout(context.Background(), applierLiveInfrastructureTimeout)
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

type applierLiveDiagnosticCommand struct {
	label string
	name  string
	args  []string
}

func applierLiveDiagnosticCommands() []applierLiveDiagnosticCommand {
	return []applierLiveDiagnosticCommand{
		{label: "helm status", name: "helm", args: []string{"status", applierLiveRelease}},
		{label: "helm history", name: "helm", args: []string{"history", applierLiveRelease}},
		{label: "pod readiness", name: "kubectl", args: []string{"get", "pods", "-o", "wide"}},
		{label: "events", name: "kubectl", args: []string{"get", "events", "--sort-by=.metadata.creationTimestamp"}},
		{label: "applier logs", name: "kubectl", args: []string{
			"logs", "-l", "app.kubernetes.io/component=applier", "--tail=60",
		}},
	}
}

// collectApplierLiveDiagnostics gives the whole bundle one deadline rather than
// multiplying a timeout by the number of commands. Failures are evidence too:
// every section is emitted, and commands reached after the budget expires see
// an already-cancelled context and return immediately with the default runner.
func collectApplierLiveDiagnostics(run applierLiveCommandRunner, timeout time.Duration) string {
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

// runApplierLiveApplyStep preflights both API paths, preserves the existing
// semantic operation, and rechecks infrastructure only when that operation
// fails. A healthy recheck makes the original failure semantic; an unhealthy
// recheck prevents a cluster outage from masquerading as a machine/Helm defect.
func runApplierLiveApplyStep(
	run applierLiveCommandRunner,
	step string,
	operation func() error,
) error {
	if err := checkApplierLiveInfrastructure(run); err != nil {
		return err
	}
	if err := operation(); err != nil {
		infrastructureErr := checkApplierLiveInfrastructure(run)
		diagnostics := collectApplierLiveDiagnostics(run, applierLiveDiagnosticsTimeout)
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
