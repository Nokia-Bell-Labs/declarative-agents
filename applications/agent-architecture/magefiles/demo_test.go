// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentArchitectureDemoReservesNoIngressPorts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "helm", "ci", "kind-demo-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	if !strings.Contains(config, "kindest/node:v1.36.1@sha256:") {
		t.Fatal("demo config does not pin the kind node image")
	}
	for _, unwanted := range []string{"ingress-ready", "hostPort:", "extraPortMappings:"} {
		if strings.Contains(config, unwanted) {
			t.Errorf("port-forward-only demo config retains %q", unwanted)
		}
	}
}
