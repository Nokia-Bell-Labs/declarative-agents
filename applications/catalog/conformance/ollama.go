// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"flag"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	defaultLiveConformanceTimeout = 5 * time.Minute
	liveConformanceFlag           = "live"
	liveConformanceTimeoutFlag    = "live-timeout"
)

var (
	liveConformance = flag.Bool(liveConformanceFlag, false,
		"enable conformance tests that perform live model inference")
	liveConformanceTimeout = flag.Duration(liveConformanceTimeoutFlag, defaultLiveConformanceTimeout,
		"per-run timeout for live model inference")
)

// ollamaBaseURL is the default local Ollama endpoint used for live inference
// wire calls (agents/*/llm/default.yaml provider_url). Model availability is
// probed through the `ollama list` CLI instead of this URL (GH-1389), so a
// provider_url change no longer silently defeats the live-conformance gate.
const ollamaBaseURL = "http://localhost:11434"

// RequireLiveModel first enforces the explicit live-conformance opt-in, then
// checks that the exact model required by the test is available. It returns the
// configured timeout for the live request or agent run.
func RequireLiveModel(t *testing.T, model string) time.Duration {
	t.Helper()
	timeout, skip, err := liveModelGate(
		*liveConformance,
		*liveConformanceTimeout,
		model,
		probeOllama,
	)
	if err != nil {
		t.Fatal(err)
	}
	if skip != "" {
		t.Skip(skip)
	}
	return timeout
}

func liveModelGate(optIn bool, timeout time.Duration, model string, probe func(string) error) (time.Duration, string, error) {
	if !optIn {
		return 0, "live model conformance disabled; run `mage liveConformance`", nil
	}

	if timeout <= 0 {
		return 0, "", fmt.Errorf("-%s must be a positive Go duration (for example 5m)", liveConformanceTimeoutFlag)
	}
	if err := probe(model); err != nil {
		return 0, fmt.Sprintf(
			"live model conformance enabled but dependency unavailable: %v; install/start the exact dependency and rerun `mage liveConformance`",
			err,
		), nil
	}
	return timeout, "", nil
}

// probeOllama checks model availability through the `ollama list` CLI. Like the
// dolt helper, the CLI's own errors answer both "is Ollama reachable" and "is
// the exact model pulled", so the probe no longer depends on a hardcoded URL
// whose only tie to the declared provider_url was a comment (GH-1389).
func probeOllama(model string) error {
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("cannot find the Ollama CLI on PATH: %w", err)
	}
	out, err := exec.Command("ollama", "list").CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot reach Ollama via `ollama list`: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return ollamaListRequires(string(out), model)
}

// ollamaListRequires reports nil when `ollama list` output includes the exact
// model tag. Untagged model names are matched against the `:latest` tag that
// Ollama assigns by default.
func ollamaListRequires(listOutput, model string) error {
	want := model
	if !strings.Contains(want, ":") {
		want += ":latest"
	}
	for _, line := range strings.Split(listOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if name := fields[0]; name == model || name == want {
			return nil
		}
	}
	return fmt.Errorf("the Ollama model %q is not pulled", model)
}
