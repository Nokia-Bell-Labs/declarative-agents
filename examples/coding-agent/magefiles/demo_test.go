// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodingDemoUsesPinnedIngressClusterAndRoutes(t *testing.T) {
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
	path, cleanup, err := codingDemoIngress()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	ingress, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"planner.coding.localhost",
		"planner-health.coding.localhost",
		"executor.coding.localhost",
		"critic.coding.localhost",
		"name: demo-coding-agent-planner",
	} {
		if !strings.Contains(string(ingress), want) {
			t.Errorf("demo ingress missing %q", want)
		}
	}
}
