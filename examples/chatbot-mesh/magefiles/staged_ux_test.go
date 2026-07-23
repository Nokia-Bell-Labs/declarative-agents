// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These cover what the ux contributes to the packaged chart (GH-702). Every file
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
		if strings.HasPrefix(m[1], "agents__") || strings.HasPrefix(m[1], "ux__") {
			keys = append(keys, m[1])
		}
	}
	if len(keys) == 0 {
		t.Fatal("no profile keys in the rendered ConfigMap; the staging or the key encoding changed")
	}
	return keys
}

// TestStagedUXCarriesOnlyWhatTheChartServes proves the ux contributes its
// descriptor and its built bundle and nothing else. The panel sources, the
// tsconfig, and package-lock.json are build inputs; node_modules is worse than
// noise, because esbuild's binary is over helm's 5 MiB per-file limit and fails
// the render outright, so a developer who had run npm install could not render
// the chart at all.
func TestStagedUXCarriesOnlyWhatTheChartServes(t *testing.T) {
	for _, key := range renderedProfileKeys(t) {
		if !strings.HasPrefix(key, "ux__app__") {
			continue
		}
		if !strings.HasPrefix(key, "ux__app__dist__") {
			t.Errorf("rendered ConfigMap carries %s; only the built bundle under ux/app/dist belongs in a pod", key)
		}
	}
}

// TestStagedUXCarriesTheServedBundle proves the counterpart: the chatbot's
// static_assets binding serves ux/app/dist off the profile mount, so cutting the
// staged tree must not cut the bundle with it. The panel would 404 at /ui.
func TestStagedUXCarriesTheServedBundle(t *testing.T) {
	keys := renderedProfileKeys(t)
	var index, assets bool
	for _, key := range keys {
		if key == "ux__app__dist__index.html" {
			index = true
		}
		if strings.HasPrefix(key, "ux__app__dist__assets__") {
			assets = true
		}
	}
	if !index {
		t.Error("rendered ConfigMap has no ux__app__dist__index.html; the chatbot's /ui would 404")
	}
	if !assets {
		t.Error("rendered ConfigMap has no ux__app__dist__assets__* key; the SPA would load without its bundle")
	}

	// The descriptor key is co-generated from ragUnits, so it is emitted whether
	// or not the packaging step placed the file -- assert it is served all the same.
	var descriptor bool
	for _, key := range keys {
		if key == "ux__ux.yaml" {
			descriptor = true
		}
	}
	if !descriptor {
		t.Error("rendered ConfigMap has no ux__ux.yaml; the UI descriptor is unserved")
	}
}

// TestStagedProfilesFitTheConfigMapLimit proves the staged profiles stay inside
// Kubernetes' 1 MiB ConfigMap limit. The limit applies to the object, not to a
// key, so it is the sum that matters -- and it was the ux tree that made the sum
// grow without anyone deploying anything new.
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

	var total int64
	err = filepath.Walk(filepath.Join(staged, "profiles"), func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk staged profiles: %v", err)
	}

	const configMapLimit = 1 << 20
	if total > configMapLimit {
		t.Errorf("staged profiles total %d bytes, over the %d ConfigMap limit", total, configMapLimit)
	}
	// Headroom, not a size contest: at over half the limit a single added asset
	// could push the ConfigMap over and fail the install rather than a test.
	if total > configMapLimit/2 {
		t.Errorf("staged profiles total %d bytes, over half the %d ConfigMap limit; the staged set has grown",
			total, configMapLimit)
	}
}

// TestStagedProfilesExcludeTestFixtures proves the agent rig fixtures do not
// reach a pod (GH-729). They are mock LLM and RAG definitions, scenarios, and
// their own profiles: test doubles with no runtime role. Every staged file
// becomes a ConfigMap key and a projected mount item in every agent pod, so
// shipping them means production pods mount mock service definitions and the
// ConfigMap grows with the test suite rather than with the product.
//
// This is the coupling GH-702 removed for the ux tree, reintroduced by fixtures
// that arrived after it. The assertion exists so the next fixture directory
// cannot re-enter silently.
func TestStagedProfilesExcludeTestFixtures(t *testing.T) {
	for _, key := range renderedProfileKeys(t) {
		if strings.Contains(key, "__"+stagedTestsDir+"__") {
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
	for _, agent := range []string{"chatbot", "rag-server", "coordinator", "creator", "executor"} {
		want := "agents__" + agent + "__profile.yaml"
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
