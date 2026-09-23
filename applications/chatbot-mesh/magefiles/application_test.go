// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/apprig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"gopkg.in/yaml.v3"
)

func TestApplicationRunnerTargetsSharedPlatformCoordinates(t *testing.T) {
	runner, err := chatbotApplicationRunner()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := apprig.LoadManifest(runner.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := apprig.Resolve(manifest, runner.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if runner.Binding.Cluster != kindrig.PlatformClusterName ||
		resolved.Namespace != chatbotApplicationNamespace ||
		resolved.Release != chatbotApplicationRelease ||
		resolved.BucketURL != "gs://chatbot-mesh-telemetry" {
		t.Fatalf("shared application coordinates = %+v, binding=%+v", resolved, runner.Binding)
	}
}

func TestApplicationOverridesKeepCollectorStorageAndUIInOneMap(t *testing.T) {
	resolved := apprig.Resolved{
		Application: "chatbot-mesh", Namespace: chatbotApplicationNamespace,
		BucketURL: "gs://chatbot-mesh-telemetry", ObjectPrefix: "chatbot-mesh",
	}
	assets := []externalUIAsset{
		{Component: "chatbot", ConfigMapName: "chat-ui", Checksum: "abc"},
		{Component: "collector", ConfigMapName: "collector-ui", Checksum: "def"},
	}
	var values map[string]any
	overrides := chatbotApplicationOverrides(resolved, "example/runtime:revision", assets)
	if err := yaml.Unmarshal([]byte(overrides), &values); err != nil {
		t.Fatalf("%v\n%s", err, overrides)
	}
	collector := values["collector"].(map[string]any)
	storage := collector["storage"].(map[string]any)
	if storage["backend"] != "object" ||
		storage["bucketName"] != "chatbot-mesh-telemetry" ||
		storage["namespace"] != chatbotApplicationNamespace ||
		collector["externalOTLPEndpoint"] != "" ||
		collector["uiArchiveConfigMap"] != "collector-ui" {
		t.Fatalf("collector override lost storage or UI: %#v", collector)
	}
	ingress := values["ingress"].(map[string]any)
	if ingress["host"] != chatbotApplicationHost ||
		ingress["observerHost"] != chatbotObserverHost ||
		ingress["collectorHost"] != chatbotCollectorHost {
		t.Fatalf("application ingress overrides = %#v", ingress)
	}
}

func TestApplicationIngressRoutesChatAndCollectorSeparately(t *testing.T) {
	root, err := chatbotApplicationRoot()
	if err != nil {
		t.Fatal(err)
	}
	for path, wants := range map[string][]string{
		filepath.Join(root, "helm", "templates", "chatbot.yaml"): {
			"ingress.host", "ingress.collectorHost", "port: {name: chat}", "port: {name: query}",
		},
		filepath.Join(root, "helm", "templates", "observer.yaml"): {
			"ingress.observerHost", "port: {name: monitor}",
		},
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s misses %q", path, want)
			}
		}
	}
}
