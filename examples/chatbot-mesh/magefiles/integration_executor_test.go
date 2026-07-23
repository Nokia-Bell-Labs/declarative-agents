// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the executor tracer's assertions without launching an agent
// (GH-731). The tracer itself is a mage target gated on a Go toolchain; what can
// be checked cheaply is that its assertions actually reject the things they
// name, and that its fakes classify the argv the shipped declarations construct.

// TestAssertExecutorCallsRejectsMissingAndForbidden proves the call assertions
// fail in both directions: a leg that did not invoke what it must, and a leg
// that invoked what it must not. A tracer whose assertions passed vacuously
// would report green while the machine skipped a word.
func TestAssertExecutorCallsRejectsMissingAndForbidden(t *testing.T) {
	scenario := executorScenario{
		name:        "example",
		wantCalls:   []string{"helm upgrade"},
		absentCalls: []string{"helm rollback"},
	}

	if err := assertExecutorCalls([]string{"helm upgrade chatbot-mesh /chart"}, scenario); err != nil {
		t.Fatalf("a satisfied scenario should pass: %v", err)
	}
	err := assertExecutorCalls([]string{"kubectl rollout status"}, scenario)
	if err == nil || !strings.Contains(err.Error(), "helm upgrade") {
		t.Errorf("a missing required call must fail, got %v", err)
	}
	err = assertExecutorCalls([]string{"helm upgrade", "helm rollback chatbot-mesh"}, scenario)
	if err == nil || !strings.Contains(err.Error(), "must not reach") {
		t.Errorf("a forbidden call must fail, got %v", err)
	}
}

// TestExecutorAuthorityProblemRejectsTransportAuthority proves the authority
// assertion catches an invocation carrying an endpoint, a credential, or a
// per-field --set. The executor edits values and triggers rollouts only; it
// accepts no endpoint or credential for a running agent (srd006 R2.3, R4.2) and
// constructs no --set (R2.2).
func TestExecutorAuthorityProblemRejectsTransportAuthority(t *testing.T) {
	clean := []string{
		"helm upgrade chatbot-mesh /chart --namespace default --reuse-values --atomic -f /work/overrides.yaml",
		"kubectl rollout status deployment/chatbot-mesh-chatbot --namespace default --timeout 120s",
	}
	if problem := executorAuthorityProblem(clean); problem != "" {
		t.Errorf("the shipped invocations must pass the authority check, got %q", problem)
	}

	for _, tc := range []struct{ name, call, want string }{
		{"endpoint", "helm upgrade --set ragUrl=http://rag0:18085", "http://"},
		{"bearer token", "kubectl get deployment --token Bearer abc", "--token"},
		{"kubeconfig", "kubectl rollout status --kubeconfig /etc/kube/config", "--kubeconfig"},
		{"per-field set", "helm upgrade chatbot-mesh /chart --set ragUnits[0].name=rag9", "--set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problem := executorAuthorityProblem([]string{tc.call})
			if !strings.Contains(problem, tc.want) {
				t.Errorf("problem = %q, want it to name %q", problem, tc.want)
			}
		})
	}
}

// TestExecutorScenariosCoverEveryTerminal proves the tracer walks every terminal
// both machines declare. A terminal added to a machine without a scenario here
// would ship an outcome no test ever reaches -- which is how the rollout read
// carried an unmapped outcome before GH-686.
func TestExecutorScenariosCoverEveryTerminal(t *testing.T) {
	scenarios := executorScenarios()
	var applyLegs, rolloutLegs int
	for _, scenario := range scenarios {
		if scenario.applyBody == "" {
			rolloutLegs++
			continue
		}
		applyLegs++
	}

	for _, tc := range []struct {
		machine string
		legs    int
	}{
		{"apply-machine.yaml", applyLegs},
		{"rollout-machine.yaml", rolloutLegs},
	} {
		var machine rolloutMachine
		readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), tc.machine), &machine)
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
// argv the shipped exec declarations construct and checks each lands in the leg
// the tracer expects. This is what binds the fakes to the declarations: change an
// argument in exec-declarations.yaml and a scenario would otherwise keep passing
// while priming a leg the run no longer takes.
func TestFakeScriptsClassifyTheDeclaredInvocations(t *testing.T) {
	fakes, err := newExecutorFakes()
	if err != nil {
		t.Fatalf("build fakes: %v", err)
	}
	defer fakes.cleanup()

	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), "exec-declarations.yaml"), &decls)
	args := map[string][]string{}
	for _, tool := range decls.Tools {
		args[tool.Name] = tool.Args
	}

	// Each word must land in its own leg, or a scenario priming one outcome
	// would silently prime another.
	for _, tc := range []struct{ word, binary, verb string }{
		{"helm_dry_run", "helm", "dry-run"},
		{"helm_upgrade", "helm", "upgrade"},
		{"helm_rollback", "helm", "rollback"},
		{"kubectl_rollout_poll", "kubectl", "poll"},
		{"kubectl_rollout_status", "kubectl", "verify"},
		{"kubectl_get_rollout_counts", "kubectl", "counts"},
	} {
		t.Run(tc.word, func(t *testing.T) {
			declared, ok := args[tc.word]
			if !ok {
				t.Fatalf("the executor declares no %s word", tc.word)
			}
			if err := fakes.plan(map[string]int{tc.verb: 42}, nil); err != nil {
				t.Fatal(err)
			}
			// Invoked by absolute path on the inherited environment: the fakes
			// are shell scripts and still need the ordinary tools on PATH, which
			// is also how the tracer runs them (it prepends its bin dir rather
			// than replacing PATH).
			err := exec.Command(filepath.Join(fakes.binDir, tc.binary), declared...).Run()
			// The planned exit code is the classification's signature: only the
			// leg under test was primed to fail.
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
