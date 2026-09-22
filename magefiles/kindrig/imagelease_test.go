// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
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

func TestImageLeaseBuiltImageRemovedOnLastRelease(t *testing.T) {
	docker := &fakeLeaseDocker{}
	manager := newFakeLeaseManager(t, docker)
	ref := "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:a1b2c3d4e5f6"

	lease, err := manager.acquire("/core", ref, "linux/arm64", "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if lease.Result.Reused || lease.Result.ImageID == "" {
		t.Fatalf("lease result = %#v, want newly built image", lease.Result)
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
	ref := "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:a1b2c3d4e5f6"
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
	ref := "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:a1b2c3d4e5f6"
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
	one, err := manager.acquire("/core", "agent-core:111111111111", "linux/arm64", "one")
	if err != nil {
		t.Fatal(err)
	}
	two, err := manager.acquire("/core", "agent-core:222222222222", "linux/arm64", "two")
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
	ref := "agent-core:a1b2c3d4e5f6"
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
		lease, err := manager.acquire("/core", "agent-core:a1b2c3d4e5f6", "linux/arm64", "smoke")
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
		lease, err := manager.acquire("/core", "agent-core:a1b2c3d4e5f6", "linux/arm64", "smoke")
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
	ref := "agent-core:a1b2c3d4e5f6"
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
