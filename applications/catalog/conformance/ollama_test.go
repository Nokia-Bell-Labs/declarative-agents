// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLiveModelGateDoesNotProbeWithoutExplicitOptIn(t *testing.T) {
	probes := 0
	timeout, skip, err := liveModelGate("", "", "installed:model", func(string) error {
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
	for _, want := range []string{"mage liveConformance", LiveConformanceEnv + "=1"} {
		if !strings.Contains(skip, want) {
			t.Errorf("disabled skip reason %q does not contain %q", skip, want)
		}
	}
}

func TestLiveModelGateOptInProbesExactModel(t *testing.T) {
	const model = "required:model"
	var probed string
	timeout, skip, err := liveModelGate("1", "7m", model, func(got string) error {
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
	_, skip, err := liveModelGate("1", "", "missing:model", func(string) error {
		return probeOllama(http.DefaultClient, unavailableOllama(t), "missing:model")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skip, `the Ollama model "missing:model" is not pulled`) {
		t.Fatalf("dependency skip reason = %q, want missing exact model", skip)
	}
}

func TestLiveModelGateRejectsInvalidTimeout(t *testing.T) {
	probes := 0
	_, _, err := liveModelGate("1", "eventually", "required:model", func(string) error {
		probes++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), LiveConformanceTimeoutEnv) {
		t.Fatalf("invalid timeout error = %v, want %s guidance", err, LiveConformanceTimeoutEnv)
	}
	if probes != 0 {
		t.Fatalf("dependency probe ran %d times with invalid timeout", probes)
	}
}

func unavailableOllama(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"installed:model"}]}`))
	}))
	t.Cleanup(server.Close)
	return server.URL
}
