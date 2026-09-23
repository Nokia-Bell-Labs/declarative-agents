// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"
)

// The GH-2535 failure: the derived Ollama image loaded, vanished from the
// shared node, and the pod sat in ErrImageNeverPull for the whole rollout.

const derivedOllama = "localhost/declarative-agents/derived/ollama:upstream-0.34.2-kind-trusted-recipe-04673e24d9d6-linux-arm64"

func TestDependencyPresenceReloadsAnImageThatVanished(t *testing.T) {
	onNode := map[string]bool{"docker.io/library/busybox:1.36": true}
	var reloaded []string

	err := ensureDependencyImagesPresent(
		[]chartDependency{{Cluster: "docker.io/library/busybox:1.36"}, {Cluster: derivedOllama}},
		func(reference string) (bool, error) { return onNode[reference], nil },
		func(spec chartDependency) error {
			reloaded = append(reloaded, spec.Cluster)
			onNode[spec.Cluster] = true
			return nil
		})

	if err != nil {
		t.Fatalf("err = %v, want the second load to restore the image", err)
	}
	if len(reloaded) != 1 || reloaded[0] != derivedOllama {
		t.Fatalf("reloaded = %v, want only the missing derived image", reloaded)
	}
}

func TestDependencyPresenceFailsByNameWhenAReloadDoesNotStick(t *testing.T) {
	err := ensureDependencyImagesPresent(
		[]chartDependency{{Cluster: derivedOllama}},
		func(string) (bool, error) { return false, nil },
		func(chartDependency) error { return nil })

	if err == nil || !strings.Contains(err.Error(), derivedOllama) {
		t.Fatalf("err = %v, want a failure naming %s", err, derivedOllama)
	}
}
