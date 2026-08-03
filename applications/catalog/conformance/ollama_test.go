// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"strings"
	"testing"
	"time"
)

func TestLiveModelGateDoesNotProbeWithoutExplicitOptIn(t *testing.T) {
	t.Parallel()
	probes := 0
	timeout, skip, err := liveModelGate(false, defaultLiveConformanceTimeout, "installed:model", func(string) error {
		probes++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if probes != 0 {
		t.Fatalf("dependency probe ran %d times without explicit opt-in", probes)
	}
	if timeout != 0 {
		t.Fatalf("disabled live timeout = %s, want zero", timeout)
	}
	if want := "mage liveConformance"; !strings.Contains(skip, want) {
		t.Errorf("disabled skip reason %q does not contain %q", skip, want)
	}
}

func TestLiveModelGateOptInProbesExactModel(t *testing.T) {
	t.Parallel()
	const model = "required:model"
	var probed string
	timeout, skip, err := liveModelGate(true, 7*time.Minute, model, func(got string) error {
		probed = got
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if skip != "" {
		t.Fatalf("enabled live gate skipped: %s", skip)
	}
	if probed != model {
		t.Fatalf("probed model = %q, want exact configured model %q", probed, model)
	}
	if timeout != 7*time.Minute {
		t.Fatalf("live timeout = %s, want 7m", timeout)
	}
}

func TestLiveModelGateOptInStillRequiresDependency(t *testing.T) {
	t.Parallel()
	const installed = "NAME              ID            SIZE     MODIFIED\ninstalled:model   abc123        1.0 GB   2 days ago\n"
	_, skip, err := liveModelGate(true, defaultLiveConformanceTimeout, "missing:model", func(string) error {
		return ollamaListRequires(installed, "missing:model")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skip, `the Ollama model "missing:model" is not pulled`) {
		t.Fatalf("dependency skip reason = %q, want missing exact model", skip)
	}
}

func TestOllamaListRequiresMatchesExactAndLatest(t *testing.T) {
	t.Parallel()
	const listing = "NAME              ID            SIZE     MODIFIED\n" +
		"qwen2.5:7b        abc123        4.7 GB   2 days ago\n" +
		"llama3.2:latest   def456        2.0 GB   1 week ago\n"
	cases := []struct {
		name    string
		model   string
		wantErr bool
	}{
		{"exact tag", "qwen2.5:7b", false},
		{"untagged resolves to latest", "llama3.2", false},
		{"tagged latest", "llama3.2:latest", false},
		{"missing model", "mistral:7b", true},
		{"untagged missing", "mistral", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ollamaListRequires(listing, tc.model)
			if tc.wantErr && err == nil {
				t.Fatalf("model %q: want not-pulled error, got nil", tc.model)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("model %q: want nil, got %v", tc.model, err)
			}
		})
	}
}

func TestLiveModelGateRejectsInvalidTimeout(t *testing.T) {
	t.Parallel()
	probes := 0
	_, _, err := liveModelGate(true, 0, "required:model", func(string) error {
		probes++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "-"+liveConformanceTimeoutFlag) {
		t.Fatalf("invalid timeout error = %v, want -%s guidance", err, liveConformanceTimeoutFlag)
	}
	if probes != 0 {
		t.Fatalf("dependency probe ran %d times with invalid timeout", probes)
	}
}
