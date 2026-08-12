// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the applier tracer's assertions without launching an agent. The tracer
// itself is a mage target gated on a Go toolchain and the agent-core checkout; what
// can be checked cheaply is that its assertions actually reject the things they name,
// and that its fakes classify the argv the shipped declarations construct.

func TestApplierHostPortsAreApplicationOwned(t *testing.T) {
	for name, endpoint := range map[string]string{
		"apply":   applierApplyURL,
		"rollout": applierRolloutURL,
		"control": applierControlHealthURL,
		"exit":    applierControlExitURL,
	} {
		if strings.Contains(endpoint, ":18230") ||
			strings.Contains(endpoint, ":18231") {
			t.Errorf("%s endpoint overlaps Coding Agent ownership: %s", name, endpoint)
		}
		if !strings.Contains(endpoint, ":1833") {
			t.Errorf("%s endpoint is outside Agent Architecture range: %s", name, endpoint)
		}
	}
}

// TestAssertApplierCallsRejectsMissingAndForbidden proves the call assertions fail in
// both directions: a leg that did not invoke what it must, and a leg that invoked
// what it must not.
func TestAssertApplierCallsRejectsMissingAndForbidden(t *testing.T) {
	scenario := applierScenario{
		name:        "example",
		wantCalls:   []string{"helm upgrade"},
		absentCalls: []string{"helm rollback"},
	}

	if err := assertApplierCalls([]string{"helm upgrade agent-architecture /chart"}, scenario); err != nil {
		t.Fatalf("a satisfied scenario should pass: %v", err)
	}
	err := assertApplierCalls([]string{"kubectl rollout status"}, scenario)
	if err == nil || !strings.Contains(err.Error(), "helm upgrade") {
		t.Errorf("a missing required call must fail, got %v", err)
	}
	err = assertApplierCalls([]string{"helm upgrade", "helm rollback agent-architecture"}, scenario)
	if err == nil || !strings.Contains(err.Error(), "must not reach") {
		t.Errorf("a forbidden call must fail, got %v", err)
	}
}

// TestApplierAuthorityProblemRejectsTransportAuthority proves the authority assertion
// catches an invocation carrying an endpoint, a credential, or a per-field --set. The
// applier edits values and triggers rollouts only; it accepts no endpoint or
// credential for a running agent (srd002-applier R2.3, R4.2) and constructs no --set
// (R2.2).
func TestApplierAuthorityProblemRejectsTransportAuthority(t *testing.T) {
	clean := []string{
		"helm upgrade agent-architecture /chart --namespace default --reuse-values -f /work/overrides.yaml",
		"kubectl rollout status deployment/agent-architecture-collector --namespace default --timeout 120s",
	}
	if problem := applierAuthorityProblem(clean); problem != "" {
		t.Errorf("the shipped invocations must pass the authority check, got %q", problem)
	}

	for _, tc := range []struct{ name, call, want string }{
		{"endpoint", "helm upgrade --set collectorUrl=http://collector:18193", "http://"},
		{"bearer token", "kubectl get deployment --token Bearer abc", "--token"},
		{"kubeconfig", "kubectl rollout status --kubeconfig /etc/kube/config", "--kubeconfig"},
		{"per-field set", "helm upgrade agent-architecture /chart --set collector.resources.requests.memory=96Mi", "--set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problem := applierAuthorityProblem([]string{tc.call})
			if !strings.Contains(problem, tc.want) {
				t.Errorf("problem = %q, want it to name %q", problem, tc.want)
			}
		})
	}
}

// TestApplierScenariosCoverEveryTerminal proves the tracer walks every terminal both
// machines declare. A terminal added to a machine without a scenario here would ship
// an outcome no test ever reaches.
func TestApplierScenariosCoverEveryTerminal(t *testing.T) {
	scenarios := applierScenarios()
	var applyLegs, rolloutLegs int
	for _, scenario := range scenarios {
		if scenario.applyBody != "" {
			applyLegs++
		} else {
			rolloutLegs++
		}
	}

	for _, tc := range []struct {
		machine string
		legs    int
	}{
		{"apply-machine.yaml", applyLegs},
		{"rollout-machine.yaml", rolloutLegs},
	} {
		var machine rolloutMachine
		readIntakeYAML(t, filepath.Join("..", "..", "catalog", "agents", "applier", tc.machine), &machine)
		if len(machine.TerminalStates) == 0 {
			t.Fatalf("%s declares no terminal states", tc.machine)
		}
		if tc.legs != len(machine.TerminalStates) {
			t.Errorf("%s declares %d terminal states (%v) but the tracer walks %d legs; every outcome needs one",
				tc.machine, len(machine.TerminalStates), machine.TerminalStates, tc.legs)
		}
	}
}

// TestFakeScriptsClassifyTheDeclaredInvocations runs the generated fakes over the
// argv the shipped exec declarations construct and checks each lands in the leg the
// tracer expects. This is what binds the fakes to the declarations: change an
// argument in exec-declarations.yaml and a scenario would otherwise keep passing while
// priming a leg the run no longer takes.
func TestFakeScriptsClassifyTheDeclaredInvocations(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	fakes, err := newApplierFakes()
	if err != nil {
		t.Fatalf("build fakes: %v", err)
	}
	defer fakes.cleanup()

	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "applier"), "exec-declarations.yaml"), &decls)
	args := map[string][]string{}
	for _, tool := range decls.Tools {
		args[tool.Name] = tool.Args
	}

	for _, tc := range []struct{ word, binary, verb string }{
		{"helm_dry_run", "helm", "dry-run"},
		{"helm_upgrade", "helm", "upgrade"},
		{"helm_rollback", "helm", "rollback"},
		{"kubectl_rollout_poll", "kubectl", "poll"},
		{"verify_rollout", "kubectl", "verify"},
		{"kubectl_get_rollout_counts", "kubectl", "counts"},
	} {
		t.Run(tc.word, func(t *testing.T) {
			declared, ok := args[tc.word]
			if !ok {
				t.Fatalf("the applier declares no %s word", tc.word)
			}
			if err := fakes.plan(map[string]int{tc.verb: 42}, nil); err != nil {
				t.Fatal(err)
			}
			// Invoked by absolute path on the inherited environment: the fakes are
			// shell scripts and still need the ordinary tools on PATH, which is also
			// how the tracer runs them (it prepends its bin dir).
			err := exec.Command(filepath.Join(fakes.binDir, tc.binary), declared...).Run()
			// The planned exit code is the classification's signature: only the leg
			// under test was primed to fail.
			if code := exitCodeOf(err); code != 42 {
				t.Errorf("%s classified as some other leg (exit %d, want the planned 42)", tc.word, code)
			}
		})
	}
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
