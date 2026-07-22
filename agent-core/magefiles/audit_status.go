// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os/exec"
	"strings"
)

// exitMachineFailed is the agent's exit code for a run whose machine reached a
// failure terminal, as distinct from 1, which means the binary could not
// complete a run at all (srd018-cli-flag-contract R6).
const exitMachineFailed = 2

// agentRunCompleted reports whether an agent invocation completed a run, which
// includes a run whose machine reached a failure terminal. A jurist audit over
// a corpus with findings is exactly that: a completed run reporting failure,
// not a broken invocation, so callers that read the outcome from the report
// must not treat its exit code as an infrastructure error.
func agentRunCompleted(runErr error) bool {
	if runErr == nil {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode() == exitMachineFailed
	}
	return false
}

func auditRunFailed(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "terminal state: failed") {
			return true
		}
		if strings.Contains(line, "run complete: status=failed") {
			return true
		}
	}
	return false
}

func envWithDefault(env []string, name, value string) []string {
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return env
		}
	}
	return append(env, prefix+value)
}
