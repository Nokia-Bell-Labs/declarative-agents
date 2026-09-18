// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestBenchMachineIsServeTemplateInstance proves the bench cleave took every
// bench-specific state out of the host (GH-2168, GH-2188): an instance cannot
// add a state, so a bench machine that still needed one would have to declare
// it here, and this test would fail.
func TestBenchMachineIsServeTemplateInstance(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(ProfilesRoot(), "agents", "bench", "machine.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var machine map[string]interface{}
	if err := yaml.Unmarshal(data, &machine); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"states", "signals", "transitions", "terminal_states", "initial_state", "budget"} {
		if _, declared := machine[key]; declared {
			t.Errorf("bench machine.yaml declares %q; the serve template owns the lifecycle", key)
		}
	}
	instances, _ := machine["instantiate"].([]interface{})
	if len(instances) != 1 {
		t.Fatalf("bench machine.yaml instantiates %d fragments, want 1", len(instances))
	}
	instance, _ := instances[0].(map[string]interface{})
	const template = "/opt/agent-core/tools/machines/serve-machine-template.yaml"
	if instance["fragment"] != template {
		t.Errorf("bench machine.yaml instantiates %v, want %s", instance["fragment"], template)
	}
}
