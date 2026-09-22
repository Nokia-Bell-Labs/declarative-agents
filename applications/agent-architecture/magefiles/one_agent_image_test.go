// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// renderedWorkload is the slice of a rendered manifest the one-image invariant
// reads: the containers and init containers of a Deployment or StatefulSet.
type renderedWorkload struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers     []renderedContainer `yaml:"containers"`
				InitContainers []renderedContainer `yaml:"initContainers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type renderedContainer struct {
	Name  string   `yaml:"name"`
	Image string   `yaml:"image"`
	Args  []string `yaml:"args"`
}

// isAgentWorkloadContainer mirrors the chart-conformance R9 classifier
// (GH-2494): a container is an agent workload when it runs the agent binary,
// which every agent role invokes with a --profile argument.
func isAgentWorkloadContainer(c renderedContainer) bool {
	return containsString(c.Args, "--profile")
}

// collectAgentImages returns the image of every --profile-bearing container in a
// rendered multi-document manifest, keyed by "workload/container" for messages.
func collectAgentImages(t *testing.T, render string) map[string]string {
	t.Helper()
	images := map[string]string{}
	decoder := yaml.NewDecoder(strings.NewReader(render))
	for {
		var workload renderedWorkload
		err := decoder.Decode(&workload)
		if err != nil {
			break
		}
		if workload.Kind != "Deployment" && workload.Kind != "StatefulSet" {
			continue
		}
		containers := append([]renderedContainer{},
			workload.Spec.Template.Spec.Containers...)
		containers = append(containers, workload.Spec.Template.Spec.InitContainers...)
		for _, container := range containers {
			if isAgentWorkloadContainer(container) {
				key := workload.Metadata.Name + "/" + container.Name
				images[key] = container.Image
			}
		}
	}
	return images
}

const agentArchitectureRootImage = "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0"

// TestEveryAgentRoleRendersRootImage proves the one-image invariant (srd005 R9,
// GH-2494): the curator, collector, and applier main containers all render the
// one application image, with no per-role or collector/applier image map. The
// applier's helm and kubectl still arrive from the CLI donor, a genuinely
// separate product image that is not an agent workload.
func TestEveryAgentRoleRendersRootImage(t *testing.T) {
	chart := preparedApplierChart(t)
	render := helmTemplate(t, chart, "--set", "applier.enabled=true")

	agentImages := collectAgentImages(t, render)
	if len(agentImages) < 3 {
		t.Fatalf("agent-workload containers = %v, want at least curator, collector, applier", agentImages)
	}
	for key, image := range agentImages {
		if image != agentArchitectureRootImage {
			t.Errorf("%s renders %q, want the one application image %q",
				key, image, agentArchitectureRootImage)
		}
	}

	// The CLI donor is a distinct third-party product, delivered to the applier
	// by an init container, and must stay independently pinned.
	if !strings.Contains(render, "alpine/k8s") {
		t.Error("applier render missing the pinned CLI donor image (alpine/k8s)")
	}
}
