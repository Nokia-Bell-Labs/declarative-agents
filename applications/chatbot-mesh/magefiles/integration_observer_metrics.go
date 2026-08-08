// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"time"
)

type observerContainerUsage struct {
	CPU    string
	Memory string
}

type observerPodMetric struct {
	Name       string
	Containers []observerContainerUsage
}

// assertObserverFleetPodMetrics waits until the observer reports a populated
// PodMetrics item for every discovered agent. Merely finding any metric is not
// enough: metrics-server also reports kube-system and infrastructure pods, while
// srd008 R6.1 requires the cards for the discovered agents.
func assertObserverFleetPodMetrics(monitorURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		labels, err := observerFleetLabelsView(monitorURL)
		if err == nil {
			var cycle observerFleetCycle
			cycle, err = parseObserverFleetCycle(labels)
			if err == nil {
				var metrics map[string]observerPodMetric
				metrics, err = observerPodMetricsFromLabels(labels)
				if err == nil {
					err = requireMetricsForObserverPods(cycle.Pods, metrics)
					if err == nil {
						fmt.Printf("helmSmoke: observer PodMetrics PASS - %d agent pods carry CPU and memory\n",
							len(cycle.Pods))
						return nil
					}
				}
			}
		}
		last = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("observer PodMetrics timed out after %s: %w", timeout, last)
}

func observerPodMetricsFromLabels(
	labels map[string]interface{},
) (map[string]observerPodMetric, error) {
	output, ok := observerLabelOutput(labels, "poll_pod_metrics")
	if !ok {
		return nil, fmt.Errorf("poll_pod_metrics has no output")
	}
	mapped, ok := output["mapped"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("poll_pod_metrics output has no mapped object")
	}
	raw, ok := mapped["pod_metrics"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("poll_pod_metrics has no pod_metrics array")
	}
	metrics := make(map[string]observerPodMetric, len(raw))
	for _, value := range raw {
		metric, err := decodeObserverPodMetric(value)
		if err != nil {
			return nil, err
		}
		if _, duplicate := metrics[metric.Name]; duplicate {
			return nil, fmt.Errorf("duplicate PodMetrics item %q", metric.Name)
		}
		metrics[metric.Name] = metric
	}
	return metrics, nil
}

func decodeObserverPodMetric(value interface{}) (observerPodMetric, error) {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return observerPodMetric{}, fmt.Errorf("PodMetrics item is %T, want object", value)
	}
	metadata, _ := raw["metadata"].(map[string]interface{})
	metric := observerPodMetric{Name: stringValue(metadata["name"])}
	if metric.Name == "" {
		return metric, fmt.Errorf("PodMetrics item has no metadata.name")
	}
	containers, ok := raw["containers"].([]interface{})
	if !ok || len(containers) == 0 {
		return metric, fmt.Errorf("PodMetrics item %q has no containers", metric.Name)
	}
	for _, value := range containers {
		container, _ := value.(map[string]interface{})
		usage, _ := container["usage"].(map[string]interface{})
		item := observerContainerUsage{
			CPU: stringValue(usage["cpu"]), Memory: stringValue(usage["memory"]),
		}
		if item.CPU == "" || item.Memory == "" {
			return metric, fmt.Errorf("PodMetrics item %q has empty CPU or memory", metric.Name)
		}
		metric.Containers = append(metric.Containers, item)
	}
	return metric, nil
}

func requireMetricsForObserverPods(
	pods map[string]observerFleetPod,
	metrics map[string]observerPodMetric,
) error {
	if len(pods) == 0 {
		return fmt.Errorf("observer discovered no agent pods")
	}
	for name := range pods {
		metric, ok := metrics[name]
		if !ok {
			return fmt.Errorf("observer has no PodMetrics for %q", name)
		}
		if len(metric.Containers) == 0 {
			return fmt.Errorf("observer PodMetrics for %q has no container usage", name)
		}
	}
	return nil
}
