// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
)

// ValidatorSpec is one validator machine to run to completion.
type ValidatorSpec struct {
	Name      string   `yaml:"name"`
	Profile   string   `yaml:"profile"`
	Directory string   `yaml:"directory,omitempty"`
	Request   string   `yaml:"request,omitempty"`
	Env       []string `yaml:"env,omitempty"`
}

// ValidatorOutcome is one validator's result. TimedOut is reported rather than
// the validator being omitted, so a hung validator is visible (srd040 R4.4).
type ValidatorOutcome struct {
	Name     string `json:"name"`
	Profile  string `json:"profile"`
	ExitCode int    `json:"exit_code"`
	Passed   bool   `json:"passed"`
	TimedOut bool   `json:"timed_out"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

// RunValidators runs every validator concurrently to completion through the
// shared child-agent path and returns one outcome per validator, keyed by
// name and sorted for determinism. The overall timeout bounds each child.
func RunValidators(ctx context.Context, binary string, specs []ValidatorSpec, timeout time.Duration) []ValidatorOutcome {
	if timeout <= 0 {
		timeout = defaultRunTimeout
	}
	outcomes := make([]ValidatorOutcome, len(specs))

	var wg sync.WaitGroup
	for i, spec := range specs {
		wg.Add(1)
		go func(slot int, spec ValidatorSpec) {
			defer wg.Done()
			outcomes[slot] = runOneValidator(ctx, binary, spec, timeout)
		}(i, spec)
	}
	wg.Wait()

	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].Name < outcomes[j].Name })
	return outcomes
}

func runOneValidator(ctx context.Context, binary string, spec ValidatorSpec, timeout time.Duration) ValidatorOutcome {
	name := spec.Name
	if name == "" {
		name = spec.Profile
	}
	outcome := ValidatorOutcome{Name: name, Profile: spec.Profile}

	cfg := execute.Config{
		Binary:    binary,
		Profile:   spec.Profile,
		Directory: spec.Directory,
		Request:   spec.Request,
		Timeout:   timeout,
		Env:       spec.Env,
	}
	result := execute.RunAgent(ctx, cfg)

	outcome.ExitCode = result.ExitCode
	outcome.TimedOut = result.TimedOut
	outcome.Stdout = result.Stdout
	outcome.Stderr = result.Stderr
	if result.Err != nil {
		outcome.Error = result.Err.Error()
	}
	outcome.Passed = result.ExitCode == 0 && !result.TimedOut && result.Err == nil
	return outcome
}

// AllPassed reports whether every validator passed, which is how a scenario
// verdict is derived (srd018 R6.1).
func AllPassed(outcomes []ValidatorOutcome) bool {
	for _, outcome := range outcomes {
		if !outcome.Passed {
			return false
		}
	}
	return len(outcomes) > 0
}

// FirstFailure names the first failing validator so a verdict can report its
// cause rather than only a boolean (srd018 R6.2).
func FirstFailure(outcomes []ValidatorOutcome) (ValidatorOutcome, bool) {
	for _, outcome := range outcomes {
		if !outcome.Passed {
			return outcome, true
		}
	}
	return ValidatorOutcome{}, false
}
