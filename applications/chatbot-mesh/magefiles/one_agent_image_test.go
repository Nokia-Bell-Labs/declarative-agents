// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// meshWorkload is the slice of a rendered manifest the one-image invariant reads:
// the containers and init containers of a Deployment or StatefulSet.
type meshWorkload struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers     []meshContainer `yaml:"containers"`
				InitContainers []meshContainer `yaml:"initContainers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type meshContainer struct {
	Name  string   `yaml:"name"`
	Image string   `yaml:"image"`
	Args  []string `yaml:"args"`
}

const chatbotMeshRootImage = "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0"

// collectMeshAgentImages returns the image of every --profile-bearing container
// (the chart-conformance R9 agent-workload classifier, GH-2494) in a rendered
// multi-document manifest, keyed by "workload/container".
func collectMeshAgentImages(t *testing.T, render string) map[string]string {
	t.Helper()
	images := map[string]string{}
	decoder := yaml.NewDecoder(strings.NewReader(render))
	for {
		var workload meshWorkload
		if err := decoder.Decode(&workload); err != nil {
			break
		}
		if workload.Kind != "Deployment" && workload.Kind != "StatefulSet" {
			continue
		}
		containers := append([]meshContainer{}, workload.Spec.Template.Spec.Containers...)
		containers = append(containers, workload.Spec.Template.Spec.InitContainers...)
		for _, container := range containers {
			if containsString(container.Args, "--profile") {
				images[workload.Metadata.Name+"/"+container.Name] = container.Image
			}
		}
	}
	return images
}

// TestEveryMeshAgentRoleRendersRootImage proves the one-image invariant (srd005
// R9, GH-2494): every agent role in the mesh — chatbot, RAG servers, creator and
// control agents, observer, the agent collector, and the applier — renders the
// one application image, with no per-role or collector/applier agent image map.
// The applier's helm and kubectl arrive from the CLI donor, and Chroma, Dolt,
// and Ollama remain independently pinned third-party products.
func TestEveryMeshAgentRoleRendersRootImage(t *testing.T) {
	render := helmTemplateOutput(t, "controlPlane.enabled=true")

	agentImages := collectMeshAgentImages(t, render)
	if len(agentImages) < 4 {
		t.Fatalf("agent-workload containers = %v, want the mesh's agent roles", agentImages)
	}
	for key, image := range agentImages {
		if image != chatbotMeshRootImage {
			t.Errorf("%s renders %q, want the one application image %q",
				key, image, chatbotMeshRootImage)
		}
	}

	// Agent mode deploys no contrib OTel collector; that image is the one
	// permitted non-agent collector product and only appears under contrib mode.
	if strings.Contains(render, "opentelemetry-collector-contrib") {
		t.Error("default agent-mode render unexpectedly contains the contrib collector image")
	}
	// Genuine third-party products stay independently pinned, not folded into
	// the one agent image.
	for _, want := range []string{"chromadb/chroma", "dolthub/dolt-sql-server", "alpine/k8s"} {
		if !strings.Contains(render, want) {
			t.Errorf("render missing independently pinned third-party image %q", want)
		}
	}
}
