// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"
)

func TestParseObserverFleetCycleAcceptsCoherentHealthySnapshot(t *testing.T) {
	labels := observerFleetFixture(false)
	cycle, err := parseObserverFleetCycle(labels)
	if err != nil {
		t.Fatal(err)
	}
	if err := assertHealthyObserverCycle(cycle, cycle.Pods); err != nil {
		t.Fatal(err)
	}
	if len(cycle.Pods) != 2 {
		t.Fatalf("pods = %d, want 2", len(cycle.Pods))
	}
	for _, label := range observerFanInLabels {
		if len(cycle.Joins[label]) != 2 {
			t.Errorf("%s items = %d, want 2", label, len(cycle.Joins[label]))
		}
	}
}

func TestParseObserverFleetCycleRejectsMixedSnapshot(t *testing.T) {
	labels := observerFleetFixture(false)
	labels["agent_state_fanin"].(map[string]interface{})["iteration"] = float64(9)

	_, err := parseObserverFleetCycle(labels)
	if err == nil || !strings.Contains(err.Error(), "mixed observer cycle") {
		t.Fatalf("error = %v, want mixed-cycle rejection", err)
	}
}

func TestParseObserverFleetCycleRejectsDuplicateJoinItem(t *testing.T) {
	labels := observerFleetFixture(false)
	output := labels["agent_tools_fanin"].(map[string]interface{})["output"].(map[string]interface{})
	items := output["items"].([]interface{})
	output["items"] = append(items, items[0])

	_, err := parseObserverFleetCycle(labels)
	if err == nil || !strings.Contains(err.Error(), "duplicate fan-in item") {
		t.Fatalf("error = %v, want duplicate-item rejection", err)
	}
}

func TestHealthyObserverCycleRejectsWrongSelectedAuthority(t *testing.T) {
	labels := observerFleetFixture(false)
	cycle, err := parseObserverFleetCycle(labels)
	if err != nil {
		t.Fatal(err)
	}
	item := cycle.Joins["agent_machine_fanin"]["observer-0"]
	item.SelectedAuthority = "http://10.0.0.2:18082"
	cycle.Joins["agent_machine_fanin"]["observer-0"] = item

	err = assertHealthyObserverCycle(cycle, cycle.Pods)
	if err == nil || !strings.Contains(err.Error(), "authority") {
		t.Fatalf("error = %v, want authority rejection", err)
	}
}

func TestDegradedObserverCycleRequiresOnlyObserverToFail(t *testing.T) {
	cycle, err := parseObserverFleetCycle(observerFleetFixture(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := assertDegradedObserverCycle(cycle, "observer-0"); err != nil {
		t.Fatal(err)
	}
	item := cycle.Joins["agent_events_fanin"]["chatbot-0"]
	item.Signal = "CommandError"
	cycle.Joins["agent_events_fanin"]["chatbot-0"] = item
	if err := assertDegradedObserverCycle(cycle, "observer-0"); err == nil {
		t.Fatal("second degraded agent was accepted")
	}
}

func observerFleetFixture(degraded bool) map[string]interface{} {
	observerPort := "18202"
	if degraded {
		observerPort = "18082"
	}
	pods := []interface{}{
		observerPodFixture("chatbot-0", "10.0.0.1", "chatbot", "18082"),
		observerPodFixture("observer-0", "10.0.0.2", "observer", observerPort),
	}
	labels := map[string]interface{}{
		"discover_mesh_pods": observerLabelFixture(
			10, map[string]interface{}{"mapped": map[string]interface{}{"pods": pods}}),
		"poll_pod_metrics": observerLabelFixture(15, map[string]interface{}{
			"mapped": map[string]interface{}{"pod_metrics": []interface{}{}},
		}),
	}
	for index, label := range observerFanInLabels {
		items := []interface{}{
			observerItemFixture(pods[0], observerMonitorReadSignal, "http://10.0.0.1:18082"),
			observerItemFixture(pods[1], observerMonitorReadSignal, "http://10.0.0.2:"+observerPort),
		}
		if degraded {
			items[1] = observerItemFixture(pods[1], "CommandError", "")
		}
		labels[label] = observerLabelFixture(
			11+index, map[string]interface{}{"items": items})
	}
	return labels
}

func observerLabelFixture(iteration int, output map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"available": true,
		"iteration": float64(iteration),
		"output":    output,
	}
}

func observerPodFixture(name, ip, component, port string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": name,
			"labels": map[string]interface{}{
				observerComponentLabel: component,
				observerMonitorLabel:   port,
			},
		},
		"status": map[string]interface{}{"podIP": ip},
	}
}

func observerItemFixture(
	pod interface{},
	signal, authority string,
) map[string]interface{} {
	structured := map[string]interface{}{}
	if signal == observerMonitorReadSignal {
		structured["status"] = float64(200)
		structured["selected_authority"] = authority
	}
	return map[string]interface{}{
		"input": pod,
		"result": map[string]interface{}{
			"signal":            signal,
			"structured_output": structured,
		},
	}
}
