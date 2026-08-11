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
	if _, exists := edges["Reporting/CommandError"]; exists {
		t.Error("Reporting/CommandError transition is unreachable and must stay absent")
	}
}

func TestScenarioCriticDeclarationsExposeHealthRetryInputs(t *testing.T) {
	data, err := os.ReadFile(ProfilePath(filepath.Join(
		"agents", "scenario-critic", "declarations.yaml",
	)))
	if err != nil {
		t.Fatal(err)
	}
	var declarations struct {
		Tools []struct {
			Name   string `yaml:"name"`
			Errors []struct {
				Signal string `yaml:"signal"`
			} `yaml:"errors"`
			Output struct {
				Schema struct {
					Properties map[string]interface{} `yaml:"properties"`
				} `yaml:"schema"`
			} `yaml:"output"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &declarations); err != nil {
		t.Fatal(err)
	}
	for _, tool := range declarations.Tools {
		if tool.Name == "start_scenario_subject" {
			for _, field := range []string{"health_base_url", "health_path", "started_at"} {
				if _, ok := tool.Output.Schema.Properties[field]; !ok {
					t.Errorf("%s output omits %s", tool.Name, field)
				}
			}
		}
		for _, name := range []string{
			"collect_scenario_verdict", "fail_scenario_start",
			"fail_scenario_unhealthy", "fail_scenario_validators", "report_rig_session",
		} {
			if tool.Name == name {
				for _, declaredError := range tool.Errors {
					if declaredError.Signal == "CommandError" {
						t.Errorf("%s declares unreachable CommandError", name)
					}
				}
			}
		}
	}
}
