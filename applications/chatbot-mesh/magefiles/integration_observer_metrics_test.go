// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"
)

func TestObserverPodMetricsCoverEveryDiscoveredAgent(t *testing.T) {
	labels := observerFleetFixture(false)
	setObserverMetricsFixture(labels, []interface{}{
		observerMetricFixture("chatbot-0", [][2]string{{"123456n", "2048Ki"}}),
		observerMetricFixture("observer-0", [][2]string{
			{"1000u", "1Mi"}, {"500u", "512Ki"},
		}),
	})
	cycle, err := parseObserverFleetCycle(labels)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := observerPodMetricsFromLabels(labels)
	if err != nil {
		t.Fatal(err)
	}
	if err := requireMetricsForObserverPods(cycle.Pods, metrics); err != nil {
		t.Fatal(err)
	}
	if len(metrics["observer-0"].Containers) != 2 {
		t.Fatalf("observer containers = %d, want 2",
			len(metrics["observer-0"].Containers))
	}
}

func TestObserverPodMetricsRejectMissingAgent(t *testing.T) {
	labels := observerFleetFixture(false)
	setObserverMetricsFixture(labels, []interface{}{
		observerMetricFixture("chatbot-0", [][2]string{{"1m", "1Mi"}}),
	})
	cycle, err := parseObserverFleetCycle(labels)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := observerPodMetricsFromLabels(labels)
	if err != nil {
		t.Fatal(err)
	}
	err = requireMetricsForObserverPods(cycle.Pods, metrics)
	if err == nil || !strings.Contains(err.Error(), "observer-0") {
		t.Fatalf("error = %v, want missing observer metric", err)
	}
}

func TestObserverPodMetricsRejectEmptyUsageAndDuplicates(t *testing.T) {
	tests := []struct {
		name    string
		metrics []interface{}
		want    string
	}{
		{
			name: "empty usage",
			metrics: []interface{}{
				observerMetricFixture("chatbot-0", [][2]string{{"", "1Mi"}}),
			},
			want: "empty CPU or memory",
		},
		{
			name: "duplicate",
			metrics: []interface{}{
				observerMetricFixture("chatbot-0", [][2]string{{"1m", "1Mi"}}),
				observerMetricFixture("chatbot-0", [][2]string{{"2m", "2Mi"}}),
			},
			want: "duplicate PodMetrics",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			labels := observerFleetFixture(false)
			setObserverMetricsFixture(labels, test.metrics)
			_, err := observerPodMetricsFromLabels(labels)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func setObserverMetricsFixture(labels map[string]interface{}, metrics []interface{}) {
	output := labels["poll_pod_metrics"].(map[string]interface{})["output"].(map[string]interface{})
	output["mapped"] = map[string]interface{}{"pod_metrics": metrics}
}

func observerMetricFixture(name string, quantities [][2]string) map[string]interface{} {
	containers := make([]interface{}, 0, len(quantities))
	for _, quantity := range quantities {
		containers = append(containers, map[string]interface{}{
			"usage": map[string]interface{}{
				"cpu": quantity[0], "memory": quantity[1],
			},
		})
	}
	return map[string]interface{}{
		"metadata":   map[string]interface{}{"name": name},
		"containers": containers,
	}
}
