// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	observerComponentLabel = "app.kubernetes.io/component"
	observerInstanceLabel  = "app.kubernetes.io/instance"
	observerMonitorLabel   = "monitorPort"
)

type observerFleetPod struct {
	Name        string
	IP          string
	Component   string
	RagUnit     string
	MonitorPort string
}

type observerFleetItem struct {
	Pod               observerFleetPod
	Signal            string
	Status            int
	SelectedAuthority string
	Body              map[string]interface{}
}

type observerFleetCycle struct {
	Iterations map[string]int
	Pods       map[string]observerFleetPod
	Joins      map[string]map[string]observerFleetItem
}

// assertObserverHelmFleet proves the in-cluster observer reaches every agent,
// then gives only the observer pod a wrong-but-authorized monitor port and proves
// the other agents keep reporting through a complete later cycle (srd008 R2.3).
func assertObserverHelmFleet(
	run helmLLMCommandRunner,
	monitorURL, release string,
	timeout time.Duration,
) (result error) {
	actual, err := observerReleaseMonitorPods(run, release)
	if err != nil {
		return err
	}
	healthy, err := waitObserverFleetCycle(monitorURL, 0, timeout, func(c observerFleetCycle) error {
		return assertHealthyObserverCycle(c, actual)
	})
	if err != nil {
		return fmt.Errorf("healthy observer fleet: %w", err)
	}
	observer, err := fleetPodByComponent(healthy, "observer")
	if err != nil {
		return err
	}
	if err := labelObserverPod(run, observer.Name, "18082"); err != nil {
		return err
	}
	defer func() {
		result = errors.Join(
			result, labelObserverPod(run, observer.Name, observer.MonitorPort))
	}()
	_, err = waitObserverFleetCycle(
		monitorURL, healthy.Iterations["poll_pod_metrics"], timeout,
		func(c observerFleetCycle) error {
			return assertDegradedObserverCycle(c, observer.Name)
		})
	if err != nil {
		return fmt.Errorf("degraded observer fleet: %w", err)
	}
	fmt.Printf("helmSmoke: observer fan-in PASS - %d agents × %d monitor endpoints; isolated observer degradation preserved the other agents\n",
		len(healthy.Pods), len(observerFanInLabels))
	return nil
}

func waitObserverFleetCycle(
	monitorURL string,
	after int,
	timeout time.Duration,
	check func(observerFleetCycle) error,
) (observerFleetCycle, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		labels, err := observerFleetLabelsView(monitorURL)
		if err == nil {
			var cycle observerFleetCycle
			cycle, err = parseObserverFleetCycle(labels)
			if err == nil && cycle.Iterations["discover_mesh_pods"] > after {
				err = check(cycle)
				if err == nil {
					return cycle, nil
				}
			} else if err == nil {
				err = fmt.Errorf("discovery iteration %d has not advanced past %d",
					cycle.Iterations["discover_mesh_pods"], after)
			}
		}
		last = err
		time.Sleep(500 * time.Millisecond)
	}
	return observerFleetCycle{}, fmt.Errorf("timed out after %s: %w", timeout, last)
}

func parseObserverFleetCycle(labels map[string]interface{}) (observerFleetCycle, error) {
	cycle := observerFleetCycle{
		Iterations: map[string]int{},
		Pods:       map[string]observerFleetPod{},
		Joins:      map[string]map[string]observerFleetItem{},
	}
	ordered := append([]string{"discover_mesh_pods"}, observerFanInLabels...)
	ordered = append(ordered, "poll_pod_metrics")
	previous := -1
	for _, label := range ordered {
		entry, output, err := observerFleetEntry(labels, label)
		if err != nil {
			return cycle, err
		}
		iteration := int(entry["iteration"].(float64))
		if iteration <= previous {
			return cycle, fmt.Errorf("mixed observer cycle: %s iteration %d is not after %d",
				label, iteration, previous)
		}
		cycle.Iterations[label] = iteration
		previous = iteration
		if label == "discover_mesh_pods" {
			cycle.Pods, err = observerPodsFromOutput(output)
		} else if label != "poll_pod_metrics" {
			cycle.Joins[label], err = observerItemsFromOutput(output)
		}
		if err != nil {
			return cycle, fmt.Errorf("%s: %w", label, err)
		}
	}
	return cycle, nil
}

func observerFleetEntry(
	labels map[string]interface{},
	label string,
) (map[string]interface{}, map[string]interface{}, error) {
	entry, ok := labels[label].(map[string]interface{})
	if !ok || entry["available"] == false {
		return nil, nil, fmt.Errorf("fleet label %q is unavailable", label)
	}
	iteration, ok := entry["iteration"].(float64)
	if !ok || iteration < 0 {
		return nil, nil, fmt.Errorf("fleet label %q has no iteration", label)
	}
	output, ok := entry["output"].(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("fleet label %q has no output", label)
	}
	return entry, output, nil
}

func observerPodsFromOutput(output map[string]interface{}) (map[string]observerFleetPod, error) {
	mapped, ok := output["mapped"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("discovery output has no mapped object")
	}
	raw, ok := mapped["pods"].([]interface{})
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("discovery output has no pods")
	}
	pods := make(map[string]observerFleetPod, len(raw))
	for _, value := range raw {
		pod, err := decodeObserverPod(value)
		if err != nil {
			return nil, err
		}
		if _, duplicate := pods[pod.Name]; duplicate {
			return nil, fmt.Errorf("duplicate discovered pod %q", pod.Name)
		}
		pods[pod.Name] = pod
	}
	return pods, nil
}

func observerItemsFromOutput(output map[string]interface{}) (map[string]observerFleetItem, error) {
	raw, ok := output["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("fan-in output has no items")
	}
	items := make(map[string]observerFleetItem, len(raw))
	for _, value := range raw {
		item, err := decodeObserverItem(value)
		if err != nil {
			return nil, err
		}
		if _, duplicate := items[item.Pod.Name]; duplicate {
			return nil, fmt.Errorf("duplicate fan-in item for pod %q", item.Pod.Name)
		}
		items[item.Pod.Name] = item
	}
	return items, nil
}

func decodeObserverItem(value interface{}) (observerFleetItem, error) {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return observerFleetItem{}, fmt.Errorf("fan-in item is %T, want object", value)
	}
	pod, err := decodeObserverPod(raw["input"])
	if err != nil {
		return observerFleetItem{}, err
	}
	result, ok := raw["result"].(map[string]interface{})
	if !ok {
		return observerFleetItem{}, fmt.Errorf("fan-in item %q has no result", pod.Name)
	}
	item := observerFleetItem{Pod: pod}
	item.Signal, _ = result["signal"].(string)
	if structured, ok := result["structured_output"].(map[string]interface{}); ok {
		if status, ok := structured["status"].(float64); ok {
			item.Status = int(status)
		}
		item.SelectedAuthority, _ = structured["selected_authority"].(string)
		item.Body, _ = structured["body"].(map[string]interface{})
	}
	return item, nil
}

func decodeObserverPod(value interface{}) (observerFleetPod, error) {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return observerFleetPod{}, fmt.Errorf("pod is %T, want object", value)
	}
	metadata, _ := raw["metadata"].(map[string]interface{})
	labels, _ := metadata["labels"].(map[string]interface{})
	status, _ := raw["status"].(map[string]interface{})
	pod := observerFleetPod{
		Name:        stringValue(metadata["name"]),
		IP:          stringValue(status["podIP"]),
		Component:   stringValue(labels[observerComponentLabel]),
		RagUnit:     stringValue(labels["chatbot-mesh/rag-unit"]),
		MonitorPort: stringValue(labels[observerMonitorLabel]),
	}
	if pod.Name == "" || pod.IP == "" || pod.MonitorPort == "" {
		return pod, fmt.Errorf("pod is missing name, podIP, or monitorPort: %+v", pod)
	}
	return pod, nil
}

func assertHealthyObserverCycle(
	cycle observerFleetCycle,
	actual map[string]observerFleetPod,
) error {
	if err := equalObserverPodSets(cycle.Pods, actual); err != nil {
		return err
	}
	for _, label := range observerFanInLabels {
		items := cycle.Joins[label]
		if len(items) != len(cycle.Pods) {
			return fmt.Errorf("%s has %d items for %d pods", label, len(items), len(cycle.Pods))
		}
		for name, pod := range cycle.Pods {
			item, ok := items[name]
			if !ok {
				return fmt.Errorf("%s has no item for %s", label, name)
			}
			wantAuthority := "http://" + pod.IP + ":" + pod.MonitorPort
			if item.Signal != observerMonitorReadSignal || item.Status != http.StatusOK {
				return fmt.Errorf("%s/%s = %s status %d", label, name, item.Signal, item.Status)
			}
			if item.SelectedAuthority != wantAuthority {
				return fmt.Errorf("%s/%s authority = %q, want %q",
					label, name, item.SelectedAuthority, wantAuthority)
			}
		}
	}
	return nil
}

func assertDegradedObserverCycle(cycle observerFleetCycle, observerName string) error {
	if cycle.Pods[observerName].MonitorPort != "18082" {
		return fmt.Errorf("degraded discovery still reports observer port %q",
			cycle.Pods[observerName].MonitorPort)
	}
	for _, label := range observerFanInLabels {
		if len(cycle.Joins[label]) != len(cycle.Pods) {
			return fmt.Errorf("%s has %d items for %d pods",
				label, len(cycle.Joins[label]), len(cycle.Pods))
		}
		if _, ok := cycle.Joins[label][observerName]; !ok {
			return fmt.Errorf("%s has no degraded observer item", label)
		}
		for name, item := range cycle.Joins[label] {
			if _, known := cycle.Pods[name]; !known {
				return fmt.Errorf("%s has unknown pod %q", label, name)
			}
			if name == observerName {
				if item.Signal != "CommandError" {
					return fmt.Errorf("%s/%s signal = %q, want CommandError",
						label, name, item.Signal)
				}
			} else if item.Signal != observerMonitorReadSignal {
				return fmt.Errorf("%s/%s degraded with observer: %s", label, name, item.Signal)
			}
		}
	}
	return nil
}

func observerReleaseMonitorPods(
	run helmLLMCommandRunner,
	release string,
) (map[string]observerFleetPod, error) {
	selector := observerInstanceLabel + "=" + release
	out, err := run("kubectl", "get", "pods", "-l", selector, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("list release pods: %w: %s", err, strings.TrimSpace(string(out)))
	}
	var listing map[string]interface{}
	if err := jsonUnmarshal(out, &listing); err != nil {
		return nil, err
	}
	raw, _ := listing["items"].([]interface{})
	pods := map[string]observerFleetPod{}
	for _, value := range raw {
		pod, err := decodeObserverPod(value)
		if err != nil {
			continue // infrastructure pods intentionally carry no monitorPort.
		}
		pods[pod.Name] = pod
	}
	if len(pods) == 0 || len(pods) == len(raw) {
		return nil, fmt.Errorf("release pod split = %d monitor agents / %d total pods", len(pods), len(raw))
	}
	return pods, nil
}

func jsonUnmarshal(data []byte, target interface{}) error {
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode kubectl pod list: %w", err)
	}
	return nil
}

func equalObserverPodSets(got, want map[string]observerFleetPod) error {
	if len(got) != len(want) {
		return fmt.Errorf("observer discovered %d agents, release has %d monitor pods",
			len(got), len(want))
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			return fmt.Errorf("observer did not discover %q", name)
		}
	}
	return nil
}

func fleetPodByComponent(cycle observerFleetCycle, component string) (observerFleetPod, error) {
	for _, pod := range cycle.Pods {
		if pod.Component == component {
			return pod, nil
		}
	}
	return observerFleetPod{}, fmt.Errorf("fleet has no component %q", component)
}

func labelObserverPod(run helmLLMCommandRunner, name, port string) error {
	out, err := run(
		"kubectl", "label", "pod", name, observerMonitorLabel+"="+port, "--overwrite")
	if err != nil {
		return fmt.Errorf("label observer pod %s=%s: %w: %s",
			observerMonitorLabel, port, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return text
}

func sortedFleetPodNames(pods map[string]observerFleetPod) []string {
	names := make([]string, 0, len(pods))
	for name := range pods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
