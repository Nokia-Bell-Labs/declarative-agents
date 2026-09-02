// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestProvisioningPanelAndItsRouteRenderTogether keeps the panel from being
// offered without the intake it posts to.
//
// The panel submits same-origin to /provisioning, an ingress route the chart
// renders only with the control plane on, and the orchestrator behind that route
// is a control-plane workload. The UI gated on applier.enabled instead, which
// defaults on, so the chart's own defaults rendered an enabled panel whose route
// did not exist -- the shape GH-502 fixed once already (GH-214).
//
// Both flag states are rendered, because a gate can agree by accident in one.
func TestProvisioningPanelAndItsRouteRenderTogether(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)

	for _, enabled := range []string{"false", "true"} {
		out, err := exec.Command("helm", "template", "rel", chartDir,
			"--namespace", "nsy", "--set", "controlPlane.enabled="+enabled).CombinedOutput()
		if err != nil {
			t.Fatalf("helm template with controlPlane.enabled=%s: %v\n%s", enabled, err, out)
		}
		rendered := string(out)

		panel := strings.Contains(rendered, "provisioning_panel: true")
		deploymentAPI := strings.Contains(rendered, "base_path: /provisioning/api")
		// The ingress rule, not the SPA nav entry: ui.yaml also declares a
		// descriptive nav item at the same path, mirroring the route table the
		// SPA compiles in, and it says nothing about whether an intake exists.
		route := strings.Contains(rendered, "- path: /provisioning\n")
		want := enabled == "true"

		if panel != want {
			t.Errorf("controlPlane.enabled=%s renders provisioning_panel true=%v, want %v",
				enabled, panel, want)
		}
		if deploymentAPI != want {
			t.Errorf("controlPlane.enabled=%s renders the deployment_api base path=%v, want %v",
				enabled, deploymentAPI, want)
		}
		if route != want {
			t.Errorf("controlPlane.enabled=%s renders the /provisioning route=%v, want %v",
				enabled, route, want)
		}
		if panel && !route {
			t.Errorf("controlPlane.enabled=%s offers the panel with no route to post to", enabled)
		}
	}
}

// TestChatbotDeclaresNoProvisioningClient pins the deletion.
//
// The chatbot declared a provisioning-workflow-orchestrator client and a
// delegate_provision operation that nothing dispatched: no tool named the
// rest_ref, and ProvisionDelegated appeared in no machine. It could not have
// worked if it had been wired -- it omitted the `values` field the orchestrator
// requires, and its limits_ref admitted neither the orchestrator's host nor its
// port. The panel reaches the orchestrator same-origin through the ingress
// instead (GH-214).
func TestChatbotDeclaresNoProvisioningClient(t *testing.T) {
	for _, file := range []string{"rest.yaml", "request-declarations.yaml", "request-machine.yaml"} {
		body := readAgentFile(t, "chatbot", file)
		for _, dead := range []string{"delegate_provision", "ProvisionDelegated"} {
			if strings.Contains(body, dead) {
				t.Errorf("chatbot %s still declares %s, which nothing dispatches", file, dead)
			}
		}
	}
}
