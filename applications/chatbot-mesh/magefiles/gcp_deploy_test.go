// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/gcprig"
	"gopkg.in/yaml.v3"
)

// The decided GKE overrides document parses, carries the pushed registry
// image in every runtime image block, pulls rather than expecting a load,
// and names the donor mirror with its digest cleared for the overlay pin
// (eng08).
func TestGcpDeployOverridesShape(t *testing.T) {
	config := gcprig.Defaults()
	config.Project = "demo-project"
	pushed := "us-central1-docker.pkg.dev/demo-project/agents/agent-core:a1b2c3d4e5f6"
	assets := []externalUIAsset{{Component: "chatbot", ConfigMapName: "demo-chatbot-ui", Checksum: "abc123"}}

	document := gcpDeployOverrides(pushed, config, assets)
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(document), &parsed); err != nil {
		t.Fatalf("overrides do not parse: %v\n%s", err, document)
	}
	for _, want := range []string{
		`repository: "us-central1-docker.pkg.dev/demo-project/agents/agent-core"`,
		`tag: "a1b2c3d4e5f6"`,
		`pullPolicy: "IfNotPresent"`,
		`repository: "us-central1-docker.pkg.dev/demo-project/agents/cli-donor"`,
		`uiArchiveConfigMap: "demo-chatbot-ui"`,
	} {
		if !strings.Contains(document, want) {
			t.Errorf("overrides missing %s:\n%s", want, document)
		}
	}
	if strings.Contains(document, "Never") {
		t.Errorf("a GKE deploy pulls; Never belongs to kind loads:\n%s", document)
	}
	applier, ok := parsed["applier"].(map[string]any)
	if !ok {
		t.Fatalf("no single applier block:\n%s", document)
	}
	if _, ok := applier["cliDonor"]; !ok {
		t.Errorf("applier block lost the donor:\n%s", document)
	}
	if enabled, _ := applier["enabled"].(bool); !enabled {
		t.Errorf("applier not enabled:\n%s", document)
	}
}

// The staged coordinates point at the gcp overlay, not the kind one.
func TestGcpValuesFileNamesTheOverlay(t *testing.T) {
	if gcpValuesFile != "gcp-values.yaml" {
		t.Fatalf("gcpValuesFile = %q", gcpValuesFile)
	}
}

// The GKE path builds its runtime image before pushing, the way demo:up
// builds before loading: without it the deploy only works on a commit whose
// image the operator happened to build by hand (GH-2439).
func TestGcpDeployBuildsBeforePushing(t *testing.T) {
	source, err := os.ReadFile("gcp_deploy.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	build := strings.Index(text, "buildSmokeRuntimeImage")
	push := strings.Index(text, "gcprig.PushAgentCore")
	if build < 0 {
		t.Fatal("gcpDeploy does not build the runtime image")
	}
	if push < 0 || build > push {
		t.Fatal("gcpDeploy pushes before it builds")
	}
}

// A GKE deploy stages its ConfigMaps through the kubeconfig the rig wrote.
// The kind path resolves a cluster name through kind; this one must not,
// because there is no kind cluster to resolve and asking fails the deploy
// before the applier machine runs (GH-2451). A runner that fails on every
// call stands in for kind: reaching it at all is the defect.
func TestGcpDeployStagesThroughKubeconfigNotKind(t *testing.T) {
	kubeconfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(kubeconfig, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	kindCalls := 0
	cluster := deployCluster{Kubeconfig: kubeconfig, Run: func(args ...string) ([]byte, error) {
		kindCalls++
		return nil, fmt.Errorf("kind must not be called on the GKE path: %v", args)
	}}

	commands, cleanup, err := cluster.commands()
	if err != nil {
		t.Fatalf("commands: %v", err)
	}
	cleanup()

	if kindCalls != 0 {
		t.Errorf("GKE staging shelled out to kind %d times", kindCalls)
	}
	if !containsKubeconfig(commands.Command("kubectl", "get", "ns").Env, kubeconfig) {
		t.Error("staged command does not carry the rig kubeconfig")
	}
}

// The kind path is unchanged: a named cluster still resolves through kind.
func TestKindDeployStagesThroughNamedCluster(t *testing.T) {
	asked := ""
	cluster := deployCluster{Name: chatbotDemoCluster, Run: func(args ...string) ([]byte, error) {
		asked = strings.Join(args, " ")
		return nil, fmt.Errorf("stop here: the resolution is what this asserts")
	}}

	if _, _, err := cluster.commands(); err == nil {
		t.Fatal("expected the recorded runner's error")
	}
	if !strings.Contains(asked, "get kubeconfig") || !strings.Contains(asked, chatbotDemoCluster) {
		t.Errorf("kind path did not resolve the named cluster: %q", asked)
	}
}

func containsKubeconfig(env []string, path string) bool {
	for _, entry := range env {
		if entry == "KUBECONFIG="+path {
			return true
		}
	}
	return false
}
