// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeLeaseDocker struct {
	mu         sync.Mutex
	images     map[string]dockerImageMetadata
	builds     map[string]int
	removals   []string
	containers string
	ensureErr  error
}

func newFakeLeaseManager(t *testing.T, docker *fakeLeaseDocker) *imageLeaseManager {
	t.Helper()
	if docker.images == nil {
		docker.images = map[string]dockerImageMetadata{}
	}
	if docker.builds == nil {
		docker.builds = map[string]int{}
	}
	return &imageLeaseManager{
		root: filepath.Join(t.TempDir(), "leases"),
		ensure: func(_ string, image, platform string) (AgentCoreImageResult, error) {
			docker.mu.Lock()
			defer docker.mu.Unlock()
			if docker.ensureErr != nil {
				return AgentCoreImageResult{}, docker.ensureErr
			}
			item, reused := docker.images[image]
			if !reused {
				docker.builds[image]++
				item = dockerImageMetadata{
					ID: "sha256:" + strings.Repeat("a", 64),
					OS: "linux", Architecture: strings.TrimPrefix(platform, "linux/"),
				}
				docker.images[image] = item
			}
			return AgentCoreImageResult{
				Reference: image, Revision: strings.Repeat("b", 40),
				Recipe:   "sha256:" + strings.Repeat("c", 64),
				Platform: platform, ImageID: item.ID, Reused: reused,
			}, nil
		},
		inspect: func(image string) (dockerImageMetadata, bool) {
			docker.mu.Lock()
			defer docker.mu.Unlock()
			item, ok := docker.images[image]
			return item, ok
		},
		run: func(args ...string) ([]byte, error) {
			docker.mu.Lock()
			defer docker.mu.Unlock()
			if len(args) > 0 && args[0] == "ps" {
				return []byte(docker.containers), nil
			}
			if len(args) == 3 && args[0] == "image" && args[1] == "rm" {
				docker.removals = append(docker.removals, args[2])
				delete(docker.images, args[2])
				return nil, nil
			}
			return nil, errors.New("unexpected docker command")
		},
		now: time.Now, pid: func() int { return 1234 }, sleep: time.Sleep,
	}
}

func mustGitLocal(t *testing.T, role ImageRole, component, revision string) string {
	t.Helper()
	ref, err := FormatGitLocal(role, component, revision, "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func mustRecipeLocal(t *testing.T, component, recipe string) string {
	t.Helper()
	ref, err := FormatRecipeLocal(CacheRole, component, recipe, "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func mustDerivedLocal(t *testing.T, component, version, recipe string) string {
	t.Helper()
	ref, err := FormatDerivedLocal(component, version, recipe, "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestClassifyLeaseImageEveryClass(t *testing.T) {
	cases := []struct {
		ref   string
		class ImageLeaseClass
	}{
		{mustGitLocal(t, RuntimeRole, "agent-core", sampleGit), RuntimeImageClass},
		{mustGitLocal(t, TestRole, "coding-model", sampleGit), TestImageClass},
		{mustDerivedLocal(t, "ollama", "0.34.2", sampleRecipe), DerivedImageClass},
		{mustRecipeLocal(t, "ollama-models", sampleRecipe), CacheImageClass},
		{CLIDonorImage, UpstreamImageClass},
	}
	for _, tc := range cases {
		class, normalized, err := ClassifyLeaseImage(tc.ref)
		if err != nil {
			t.Fatalf("%s: %v", tc.ref, err)
		}
		if class != tc.class {
			t.Errorf("%s class = %s, want %s", tc.ref, class, tc.class)
		}
		if normalized == "" {
			t.Errorf("%s normalized empty", tc.ref)
		}
	}
	for _, bad := range []string{
		"kindrig/traefik:v3.7.10",
		"ghcr.io/nokia-bell-labs/declarative-agents/agent-core:local",
		"localhost/declarative-agents/runtime/agent-core:a1b2c3d4e5f6",
		"localhost/declarative-agents/runtime/agent-core:local",
	} {
		if _, _, err := ClassifyLeaseImage(bad); err == nil {
			t.Errorf("classified prohibited %q", bad)
		}
	}
}

func TestImageLeaseBuiltImageRemovedOnLastRelease(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	ref := mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)

	lease, err := manager.acquire("/core", ref, "linux/arm64", "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if lease.Result.Reused || lease.Result.ImageID == "" {
		t.Fatalf("lease result = %#v, want newly built image", lease.Result)
	}
	state, found, err := manager.read(ref)
	if err != nil || !found || state.Class != RuntimeImageClass {
		t.Fatalf("recorded class = %#v found %v err %v", state, found, err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 1 || docker.removals[0] != ref {
		t.Fatalf("removals = %v, want %s", docker.removals, ref)
	}
	if _, err := os.Stat(manager.path(ref)); !os.IsNotExist(err) {
		t.Fatalf("lease state survived release: %v", err)
	}
}

func TestImageLeaseNeverRemovesPreExistingTag(t *testing.T) {
	ref := mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)
	docker := &fakeLeaseDocker{images: map[string]dockerImageMetadata{
		ref: {ID: "sha256:" + strings.Repeat("a", 64), OS: "linux", Architecture: "arm64"},
	}}
	manager := newFakeLeaseManager(t, docker)
	lease, err := manager.acquire("/core", ref, "linux/arm64", "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Result.Reused {
		t.Fatal("pre-existing matching tag was not reused")
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 0 {
		t.Fatalf("removed pre-existing tag: %v", docker.removals)
	}
	if _, exists := docker.images[ref]; !exists {
		t.Fatal("pre-existing image disappeared")
	}
}

func TestImageLeaseSameTagConcurrentOwnersBuildOnceAndDeleteLast(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	ref := mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)
	leases := make(chan *AgentCoreImageLease, 2)
	errs := make(chan error, 2)
	start := make(chan struct{})
	for _, owner := range []string{"one", "two"} {
		go func(owner string) {
			<-start
			lease, err := manager.acquire("/core", ref, "linux/arm64", owner)
			leases <- lease
			errs <- err
		}(owner)
	}
	close(start)
	first, second := <-leases, <-leases
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if docker.builds[ref] != 1 {
		t.Fatalf("builds = %d, want one", docker.builds[ref])
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 0 {
		t.Fatalf("first owner removed shared image: %v", docker.removals)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 1 {
		t.Fatalf("last owner removals = %v, want one", docker.removals)
	}
}

func TestImageLeaseDifferentTagsAreIndependent(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	one, err := manager.acquire("/core",
		mustGitLocal(t, RuntimeRole, "agent-core", "111111111111"), "linux/arm64", "one")
	if err != nil {
		t.Fatal(err)
	}
	two, err := manager.acquire("/core",
		mustGitLocal(t, RuntimeRole, "agent-core", "222222222222"), "linux/arm64", "two")
	if err != nil {
		t.Fatal(err)
	}
	if err := one.Release(); err != nil {
		t.Fatal(err)
	}
	if _, exists := docker.images[two.Result.Reference]; !exists {
		t.Fatal("releasing one tag removed a different tag")
	}
	if err := two.Release(); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 2 {
		t.Fatalf("independent removals = %v", docker.removals)
	}
}

func TestImageLeaseBuildFailureCreatesNoState(t *testing.T) {
	docker := &fakeLeaseDocker{ensureErr: errors.New("build failed")}
	manager := newFakeLeaseManager(t, docker)
	ref := mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)
	if _, err := manager.acquire("/core", ref, "linux/arm64", "smoke"); err == nil {
		t.Fatal("build failure accepted")
	}
	if _, err := os.Stat(manager.path(ref)); !os.IsNotExist(err) {
		t.Fatalf("failed acquire left state: %v", err)
	}
}

func TestImageLeaseRefusesChangedOrContainerUsedImage(t *testing.T) {
	t.Run("changed ID", func(t *testing.T) {
		docker := &fakeLeaseDocker{}
		manager := newFakeLeaseManager(t, docker)
		lease, err := manager.acquire("/core",
			mustGitLocal(t, RuntimeRole, "agent-core", sampleGit), "linux/arm64", "smoke")
		if err != nil {
			t.Fatal(err)
		}
		docker.images[lease.Result.Reference] = dockerImageMetadata{
			ID: "sha256:" + strings.Repeat("d", 64), OS: "linux", Architecture: "arm64",
		}
		if err := lease.Release(); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Fatalf("changed image release = %v", err)
		}
		if len(docker.removals) != 0 {
			t.Fatalf("changed image removed: %v", docker.removals)
		}
	})
	t.Run("container use", func(t *testing.T) {
		docker := &fakeLeaseDocker{containers: "container-a\n"}
		manager := newFakeLeaseManager(t, docker)
		lease, err := manager.acquire("/core",
			mustGitLocal(t, RuntimeRole, "agent-core", sampleGit), "linux/arm64", "smoke")
		if err != nil {
			t.Fatal(err)
		}
		if err := lease.Release(); err == nil || !strings.Contains(err.Error(), "used by") {
			t.Fatalf("container-used release = %v", err)
		}
		if len(docker.removals) != 0 {
			t.Fatalf("container-used image removed: %v", docker.removals)
		}
	})
}

func TestImageLeaseStaleOwnerIsDiagnosableAndExplicitlyRecoverable(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	ref := mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)
	if _, err := manager.acquire("/core", ref, "linux/arm64", "killed-run"); err != nil {
		t.Fatal(err)
	}
	active, diagnostic := manager.status(ref)
	if !active || !strings.Contains(diagnostic, "active owner") ||
		!strings.Contains(diagnostic, "imageLeaseRecover") {
		t.Fatalf("stale diagnostic = active %v, %q", active, diagnostic)
	}
	if len(docker.removals) != 0 {
		t.Fatal("status inspection deleted a stale lease")
	}
	if err := manager.recover(ref); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 1 {
		t.Fatalf("explicit recovery removals = %v", docker.removals)
	}
}

func TestImageLeaseEveryLocalClassRemovesOnLastRelease(t *testing.T) {
	cases := []struct {
		class ImageLeaseClass
		ref   string
	}{
		{RuntimeImageClass, mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)},
		{TestImageClass, mustGitLocal(t, TestRole, "coding-model", sampleGit)},
		{DerivedImageClass, mustDerivedLocal(t, "ollama", "0.34.2", sampleRecipe)},
		{CacheImageClass, mustRecipeLocal(t, "ollama-models", sampleRecipe)},
	}
	for _, tc := range cases {
		t.Run(string(tc.class), func(t *testing.T) {
			docker := &fakeLeaseDocker{}
			manager := newFakeLeaseManager(t, docker)
			lease, err := manager.acquire("/core", tc.ref, "linux/arm64", "da-platform")
			if err != nil {
				t.Fatal(err)
			}
			state, found, err := manager.read(tc.ref)
			if err != nil || !found || state.Class != tc.class {
				t.Fatalf("class = %#v found %v err %v", state, found, err)
			}
			if err := lease.Release(); err != nil {
				t.Fatal(err)
			}
			if len(docker.removals) != 1 {
				t.Fatalf("removals = %v", docker.removals)
			}
		})
	}
}

func TestImageLeaseUpstreamClassIsNotRemovedOnLastRelease(t *testing.T) {
	normalized, err := NormalizeUpstream(CLIDonorImage)
	if err != nil {
		t.Fatal(err)
	}
	docker := &fakeLeaseDocker{images: map[string]dockerImageMetadata{
		normalized: {ID: "sha256:" + strings.Repeat("a", 64), OS: "linux", Architecture: "arm64"},
	}}
	manager := newFakeLeaseManager(t, docker)
	lease, err := manager.acquire("/core", CLIDonorImage, "linux/arm64", "da-platform")
	if err != nil {
		t.Fatal(err)
	}
	if docker.builds[normalized] != 0 {
		t.Fatal("upstream pin was rebuilt")
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 0 {
		t.Fatalf("removed upstream pin: %v", docker.removals)
	}
	if _, exists := docker.images[normalized]; !exists {
		t.Fatal("upstream pin disappeared")
	}
}

func TestImageLeaseAcquireLocalDerivedRequiresPresentImage(t *testing.T) {
	ref := mustDerivedLocal(t, "ollama", "0.34.2-kind-trusted", sampleRecipe)
	docker := &fakeLeaseDocker{images: map[string]dockerImageMetadata{
		ref: {ID: "sha256:" + strings.Repeat("d", 64), OS: "linux", Architecture: "arm64"},
	}}
	manager := newFakeLeaseManager(t, docker)
	lease, err := manager.acquireLocal(ref, "chatbot-mesh-llm-tier")
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestImageLeaseAcquireRejectsUntypedNames(t *testing.T) {
	manager := newFakeLeaseManager(t, &fakeLeaseDocker{})
	if _, err := manager.acquire("/core",
		"localhost/declarative-agents/runtime/agent-core:a1b2c3d4e5f6", "linux/arm64", "smoke"); err == nil {
		t.Fatal("untyped acquire accepted")
	}
}

func TestImageLeaseReconcileDropsStaleClusterOwners(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	manager.clusters = func() ([]string, error) { return []string{"da-platform"}, nil }
	ref := mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)
	if _, err := manager.acquire("/core", ref, "linux/arm64", "da-gone"); err != nil {
		t.Fatal(err)
	}
	if err := manager.reconcile(); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 1 {
		t.Fatalf("stale cluster last-release removals = %v", docker.removals)
	}
	if _, err := os.Stat(manager.path(ref)); !os.IsNotExist(err) {
		t.Fatalf("stale lease survived reconcile: %v", err)
	}
}

func TestImageLeaseReconcileKeepsLiveClusterAndScenarioOwners(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	manager.clusters = func() ([]string, error) { return []string{"da-platform"}, nil }
	live := mustGitLocal(t, RuntimeRole, "agent-core", "111111111111")
	scenario := mustGitLocal(t, RuntimeRole, "agent-core", "222222222222")
	if _, err := manager.acquire("/core", live, "linux/arm64", "da-platform"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.acquire("/core", scenario, "linux/arm64", "coding-agent-demo"); err != nil {
		t.Fatal(err)
	}
	if err := manager.reconcile(); err != nil {
		t.Fatal(err)
	}
	if len(docker.removals) != 0 {
		t.Fatalf("reconcile removed live owners: %v", docker.removals)
	}
	if _, err := os.Stat(manager.path(live)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manager.path(scenario)); err != nil {
		t.Fatal(err)
	}
}

func TestImageLeaseReconcileDoesNothingWhenKindListFails(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	manager.clusters = func() ([]string, error) { return nil, errors.New("kind missing") }
	ref := mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)
	if _, err := manager.acquire("/core", ref, "linux/arm64", "da-gone"); err != nil {
		t.Fatal(err)
	}
	if err := manager.reconcile(); err == nil {
		t.Fatal("failed kind list accepted")
	}
	if len(docker.removals) != 0 {
		t.Fatalf("failed kind list deleted images: %v", docker.removals)
	}
}

func TestImageLeaseMigratesV1State(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	ref := mustGitLocal(t, RuntimeRole, "agent-core", sampleGit)
	if err := os.MkdirAll(manager.root, 0o700); err != nil {
		t.Fatal(err)
	}
	v1 := imageLeaseState{
		Version: 1, Reference: ref, ImageID: "sha256:" + strings.Repeat("a", 64),
		Platform: "linux/arm64", Owners: map[string]imageLeaseOwner{
			"da-platform-p1-1-1": {PID: 1, Acquired: time.Unix(1, 0).UTC()},
		},
	}
	data, err := json.Marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.path(ref), data, 0o600); err != nil {
		t.Fatal(err)
	}
	state, found, err := manager.read(ref)
	if err != nil || !found {
		t.Fatalf("v1 read = found %v err %v", found, err)
	}
	if state.Version != imageLeaseVersion || state.Class != RuntimeImageClass {
		t.Fatalf("migrated state = %#v", state)
	}
}

func TestConfiguredUpstreamPinsAreFullyQualified(t *testing.T) {
	pins := ConfiguredUpstreamPins()
	if len(pins) == 0 {
		t.Fatal("no configured pins")
	}
	for _, pin := range pins {
		if !IsConfiguredUpstreamPin(pin) {
			t.Errorf("pin %s not recognized", pin)
		}
		if KindrigAliasDiagnosis(pin) != "" {
			t.Errorf("pin %s diagnosed as kindrig", pin)
		}
	}
	if !IsConfiguredUpstreamPin("docker.io/library/traefik:v3.7.10") {
		t.Fatal("tag-only traefik listing was not a pin")
	}
	if IsConfiguredUpstreamPin("kindrig/traefik:v3.7.10") {
		t.Fatal("kindrig alias treated as a pin")
	}
}

func TestKindrigAliasDiagnosisNamesTheProblem(t *testing.T) {
	if got := KindrigAliasDiagnosis("kindrig/cli-donor:1.31.4"); got == "" || !strings.Contains(got, "kindrig") {
		t.Fatalf("diagnosis = %q", got)
	}
	if KindrigAliasDiagnosis("docker.io/library/traefik:v3.7.10") != "" {
		t.Fatal("upstream pin diagnosed as kindrig")
	}
}
