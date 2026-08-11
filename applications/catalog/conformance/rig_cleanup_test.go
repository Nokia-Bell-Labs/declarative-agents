// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestScenarioCriticRoutesTeardownFailuresThroughEmergencyCleanup(t *testing.T) {
	data, err := os.ReadFile(ProfilePath(filepath.Join(
		"agents", "scenario-critic", "machine.yaml",
	)))
	if err != nil {
		t.Fatal(err)
	}
	var machine struct {
		Transitions []struct {
			State, Signal, Next, Action string
		} `yaml:"transitions"`
	}
	if err := yaml.Unmarshal(data, &machine); err != nil {
		t.Fatal(err)
	}
	edges := make(map[string]string)
	for _, transition := range machine.Transitions {
		key := transition.State + "/" + transition.Signal
		edges[key] = transition.Next + "/" + transition.Action
	}
	for _, key := range []string{
		"ListingChildren/CommandError",
		"TearingDown/BudgetExhausted",
		"TearingDown/CommandError",
	} {
		if got := edges[key]; got != "EmergencyTeardown/stop_all_services" {
			t.Errorf("%s = %q, want EmergencyTeardown/stop_all_services", key, got)
		}
	}
	if got := edges["EmergencyTeardown/AllServicesStopped"]; got != "Failed/" {
		t.Errorf("emergency completion = %q, want Failed/", got)
	}
}
