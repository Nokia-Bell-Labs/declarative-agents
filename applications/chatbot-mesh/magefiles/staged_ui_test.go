// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These cover what the chatbot UI contributes to the packaged chart (GH-702). Every file
// staged into profiles/ becomes a ConfigMap key and a projected mount item in
// every agent pod, so the staged set has to be what the chart consumes rather
// than whatever happens to sit in the source tree.

// configMapKeyRE matches the ConfigMap data keys in a rendered chart. Keys are
// the staged paths with "/" encoded as "__" (ConfigMap keys cannot contain "/").
var configMapKeyRE = regexp.MustCompile(`(?m)^  ([a-zA-Z0-9_.-]+):`)

// renderedProfileKeys stages the chart through the production packaging path and
// returns the profiles ConfigMap keys helm renders from it.
func renderedProfileKeys(t *testing.T) []string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	out, err := exec.Command("helm", "template", "relx", staged, "--namespace", "nsy").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	var keys []string
	for _, m := range configMapKeyRE.FindAllStringSubmatch(string(out), -1) {
		if strings.HasPrefix(m[1], "agents__") || strings.HasPrefix(m[1], "applications__") {
			keys = append(keys, m[1])
		}
	}
	if len(keys) == 0 {
		t.Fatal("no profile keys in the rendered ConfigMap; the staging or the key encoding changed")
	}
	return keys
}

// TestStagedChatbotUICarriesOnlyWhatTheChartServes proves the chatbot UI contributes its
// descriptor and its built bundle and nothing else. The panel sources, the
// tsconfig, and package-lock.json are build inputs; node_modules is worse than
// noise, because esbuild's binary is over helm's 5 MiB per-file limit and fails
// the render outright, so a developer who had run npm install could not render
// the chart at all.
func TestStagedChatbotUICarriesOnlyWhatTheChartServes(t *testing.T) {
	for _, key := range renderedProfileKeys(t) {
		if !strings.HasPrefix(key, "agents__chatbot__ui__app__") {
			continue
		}
		if !strings.HasPrefix(key, "agents__chatbot__ui__app__dist__") {
			t.Errorf("rendered ConfigMap carries %s; only the built bundle under agents/chatbot/ui/app/dist belongs in a pod", key)
		}
	}
}

// TestStagedChatbotUICarriesTheServedBundle proves the counterpart: the chatbot's
// static_assets binding serves agents/chatbot/ui/app/dist off the profile mount, so cutting the
// staged tree must not cut the bundle with it. The panel would 404 at /ui.
func TestStagedChatbotUICarriesTheServedBundle(t *testing.T) {
	keys := renderedProfileKeys(t)
	var index, assets bool
	for _, key := range keys {
		if key == "agents__chatbot__ui__app__dist__index.html" {
			index = true
		}
		if strings.HasPrefix(key, "agents__chatbot__ui__app__dist__assets__") {
			assets = true
		}
	}
	if !index {
		t.Error("rendered ConfigMap has no agents__chatbot__ui__app__dist__index.html; the chatbot's /ui would 404")
	}
	if !assets {
		t.Error("rendered ConfigMap has no agents__chatbot__ui__app__dist__assets__* key; the SPA would load without its bundle")
	}

	// The descriptor key is co-generated from ragUnits, so it is emitted whether
	// or not the packaging step placed the file -- assert it is served all the same.
	var descriptor bool
	for _, key := range keys {
		if key == "agents__chatbot__ui__ui.yaml" {
			descriptor = true
		}
	}
	if !descriptor {
		t.Error("rendered ConfigMap has no agents__chatbot__ui__ui.yaml; the UI descriptor is unserved")
	}
}

func TestStagedObserverUICarriesOnlyTheServedBundle(t *testing.T) {
	keys := renderedProfileKeys(t)
	var index, assets bool
	for _, key := range keys {
		if !strings.HasPrefix(key, "agents__observer__ui__") {
			continue
		}
		if !strings.HasPrefix(key, "agents__observer__ui__dist__") {
			t.Errorf("rendered ConfigMap carries observer build input %s; only ui/dist belongs in a pod", key)
		}
		if key == "agents__observer__ui__dist__index.html" {
			index = true
		}
		if strings.HasPrefix(key, "agents__observer__ui__dist__assets__") {
			assets = true
		}
	}
	if !index {
		t.Error("rendered ConfigMap has no agents__observer__ui__dist__index.html; the observer /ui would 404")
	}
	if !assets {
		t.Error("rendered ConfigMap has no observer dist assets; the observer shell would load without its bundle")
	}
}

func TestObserverUIUsesDedicatedConfigMapAtPreservedPath(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()
	out, err := exec.Command("helm", "template", "relx", staged, "--namespace", "nsy").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	var shared, observerUI, observerDeployment string
	for _, doc := range strings.Split(string(out), "\n---") {
		switch {
		case strings.Contains(doc, "kind: Deployment") &&
			strings.Contains(doc, "name: relx-chatbot-mesh-observer"):
			observerDeployment = doc
		case strings.Contains(doc, "name: relx-chatbot-mesh-profiles"):
			shared = doc
		case strings.Contains(doc, "name: relx-chatbot-mesh-observer-ui"):
			observerUI = doc
		}
	}
	if strings.Contains(shared, "agents__observer__ui__dist__") {
		t.Error("shared profiles ConfigMap still carries the observer UI bundle")
	}
	if !strings.Contains(observerUI, "agents__observer__ui__dist__index.html") {
		t.Error("observer-only ConfigMap is missing the UI index")
	}
	for _, contract := range []string{
		"name: observer-ui",
		"mountPath: \"/profiles/agents/observer/ui/dist\"",
		"name: relx-chatbot-mesh-observer-ui",
	} {
		if !strings.Contains(observerDeployment, contract) {
			t.Errorf("observer Deployment missing dedicated UI mount contract %q", contract)
		}
	}
}

// TestStagedProfilesFitTheConfigMapLimit proves the rendered shared profiles
// ConfigMap stays inside Kubernetes' 1 MiB limit. UI bundles mounted only into
// their serving actors use dedicated ConfigMaps and must not be charged to this
// shared object that every agent pod mounts.
func TestStagedProfilesFitTheConfigMapLimit(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	out, err := exec.Command("helm", "template", "relx", staged, "--namespace", "nsy").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	var profilesConfigMap string
	for _, doc := range strings.Split(string(out), "\n---") {
		if strings.Contains(doc, "kind: ConfigMap") &&
			strings.Contains(doc, "name: relx-chatbot-mesh-profiles") {
			profilesConfigMap = doc
			break
		}
	}
	if profilesConfigMap == "" {
		t.Fatal("rendered chart has no shared profiles ConfigMap")
	}
	total := len(profilesConfigMap)

	const configMapLimit = 1 << 20
	if total > configMapLimit {
		t.Errorf("rendered profiles ConfigMap is %d bytes, over the %d-byte limit", total, configMapLimit)
	}
	// Reserve a quarter of the object limit (256 KiB) for YAML/object overhead
	// and ordinary profile growth. The old half-limit guard left more unused
	// capacity than the entire shipped UI bundle and failed while the rendered
	// object still had roughly 50% headroom; three quarters remains a meaningful
	// early warning without treating useful ConfigMap capacity as unavailable.
	const safetyThreshold = configMapLimit * 3 / 4
	if total > safetyThreshold {
		t.Errorf("rendered profiles ConfigMap is %d bytes, over the %d-byte safety threshold for a %d-byte ConfigMap",
			total, safetyThreshold, configMapLimit)
	}
}

// TestStagedProfilesExcludeTestFixtures proves the agent rig fixtures do not
// reach a pod (GH-729). They are mock LLM and RAG definitions, scenarios, and
// their own profiles: test doubles with no runtime role. Every staged file
// becomes a ConfigMap key and a projected mount item in every agent pod, so
// shipping them means production pods mount mock service definitions and the
// ConfigMap grows with the test suite rather than with the product.
//
// This is the coupling GH-702 removed for the chatbot UI tree, reintroduced by fixtures
// that arrived after it. The assertion exists so the next fixture directory
// cannot re-enter silently.
func TestStagedProfilesExcludeTestFixtures(t *testing.T) {
	for _, key := range renderedProfileKeys(t) {
		if strings.Contains(key, "__tests__") {
			t.Errorf("rendered ConfigMap carries %s; agent test fixtures do not belong in a pod", key)
		}
	}
}

// TestStagedProfilesKeepWhatAnAgentRuns is the counterpart: pruning fixtures must
// not take a profile an agent needs. TestStagedProfilesCoverEnabledDeployments
// guards the staging list, and this guards what survives the prune -- the
// distinction matters because a prune runs after that list is satisfied.
func TestStagedProfilesKeepWhatAnAgentRuns(t *testing.T) {
	keys := renderedProfileKeys(t)
	for _, agent := range []string{"chatbot", "rag-server", "provisioning-workflow-orchestrator", "creator", "applier"} {
		want := "agents__" + agent + "__profile.yaml"
		if agent == "applier" {
			want = "applications__chatbot-mesh__applier__profile.yaml"
		}
		var found bool
		for _, key := range keys {
			if key == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no %s key in the rendered ConfigMap; the prune took a profile an agent starts with", want)
		}
	}
}
