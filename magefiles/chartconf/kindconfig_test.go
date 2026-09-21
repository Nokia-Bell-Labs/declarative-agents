// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package chartconf

import "testing"

const pinnedNode = "kindest/node:v1.31.4@sha256:" +
	"2cb39f7295fe7eafee0842b5052a095469f00e0f2f6fdc0d0bd1cd76e7b5d7e5"

func kindDocument(image string) string {
	document := "apiVersion: kind.x-k8s.io/v1alpha4\nkind: Cluster\nnodes:\n  - role: control-plane\n"
	if image != "" {
		document += "    image: " + image + "\n"
	}
	return document
}

func TestPinnedNodeImagePasses(t *testing.T) {
	findings, err := CheckKindConfig("platform-kind-config.yaml", kindDocument(pinnedNode))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no finding, got %v", findings)
	}
}

func TestUnpinnedNodeImageIsReported(t *testing.T) {
	for name, image := range map[string]string{
		"no digest":  "kindest/node:v1.31.4",
		"no version": "kindest/node@sha256:2cb39f7295fe7eafee0842b5052a095469f00e0f2f6fdc0d0bd1cd76e7b5d7e5",
		"no image":   "",
	} {
		t.Run(name, func(t *testing.T) {
			findings, err := CheckKindConfig("config.yaml", kindDocument(image))
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != 1 || findings[0].Rule != "R6.1" {
				t.Fatalf("expected one R6.1 finding, got %v", findings)
			}
		})
	}
}

// A values overlay and a cluster configuration both sit in ci/; only one is
// a cluster configuration.
func TestIsKindConfigSeparatesConfigurationFromValues(t *testing.T) {
	if !IsKindConfig(kindDocument(pinnedNode)) {
		t.Error("cluster configuration not recognised")
	}
	for name, document := range map[string]string{
		"values overlay":    "image:\n  repository: agent-core\n  tag: \"0.1.0\"\n",
		"persistent volume": "apiVersion: v1\nkind: PersistentVolume\nmetadata:\n  name: workspace\n",
		"not yaml":          "\tthis: [is: not\n",
	} {
		t.Run(name, func(t *testing.T) {
			if IsKindConfig(document) {
				t.Error("wrongly recognised as a cluster configuration")
			}
		})
	}
}
