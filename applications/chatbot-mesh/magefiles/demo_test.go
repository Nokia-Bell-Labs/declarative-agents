// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestChatbotDemoUsesPinnedIngressCluster(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "helm", "ci", "kind-demo-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	for _, want := range []string{
		"kindest/node:v1.36.1@sha256:",
		"ingress-ready=true",
		"containerPort: 80",
		"hostPort: 80",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("demo config missing %q", want)
		}
	}
	if chatbotDemoCluster != "da-chatbot-mesh-demo" ||
		chatbotDemoHost != "chatbot.localhost" ||
		chatbotDemoIngressClass != "traefik" ||
		chatbotDemoValuesFile != "kind-values.yaml" {
		t.Fatalf("demo identity = %s %s %s %s", chatbotDemoCluster,
			chatbotDemoHost, chatbotDemoIngressClass, chatbotDemoValuesFile)
	}
}

func TestChatbotDemoRequiresTheChartModelsFromHostOllama(t *testing.T) {
	models, err := demoRequiredModels(filepath.Join("..", "helm"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"qwen3-embedding:8b", "qwen2.5:3b", "ornith:9b"}
	if !slices.Equal(models, want) {
		t.Fatalf("demo models = %v, want %v", models, want)
	}
}
