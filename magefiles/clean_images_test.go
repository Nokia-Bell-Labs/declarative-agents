// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// fakeImageDaemon serves docker image ls/inspect/rm for scripted tags and
// records removals. labels maps a ref to its image labels as JSON (default null).
type fakeImageDaemon struct {
	refs    []string
	created map[string]time.Time
	labels  map[string]string
	inUse   map[string]bool
	removed []string
}

func (d *fakeImageDaemon) run(args ...string) ([]byte, error) {
	switch {
	case len(args) == 4 && args[0] == "image" && args[1] == "ls":
		return []byte(strings.Join(d.refs, "\n") + "\n"), nil
	case len(args) == 5 && args[0] == "image" && args[1] == "inspect":
		ref := args[4]
		labels := d.labels[ref]
		if labels == "" {
			labels = "null"
		}
		created := d.created[ref]
		if created.IsZero() {
			created = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		}
		return []byte(created.Format(time.RFC3339Nano) + "|" + labels + "\n"), nil
	case len(args) == 3 && args[0] == "image" && args[1] == "rm":
		if d.inUse[args[2]] {
			return []byte("image is being used by running container"), errors.New("conflict")
		}
		d.removed = append(d.removed, args[2])
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected docker %v", args)
}

func gitLocal(t *testing.T, component string, revision uint64) string {
	t.Helper()
	ref, err := kindrig.FormatGitLocal(
		kindrig.RuntimeRole, component, fmt.Sprintf("%012x", revision), "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func cacheLocal(t *testing.T, component string, recipe uint64) string {
	t.Helper()
	ref, err := kindrig.FormatRecipeLocal(
		kindrig.CacheRole, component, fmt.Sprintf("%012x", recipe), "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// newImageDaemon gives runtime/agent-core ten git identities (r0 oldest) plus
// a :local leftover, a kindrig alias, an upstream pin, and an unrelated image.
func newImageDaemon(t *testing.T) *fakeImageDaemon {
	t.Helper()
	previous := cleanImageReconcile
	cleanImageReconcile = func() error { return nil }
	t.Cleanup(func() { cleanImageReconcile = previous })

	d := &fakeImageDaemon{
		created: map[string]time.Time{},
		labels:  map[string]string{},
		inUse:   map[string]bool{},
	}
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		ref := gitLocal(t, "agent-core", 0xa00000000000+uint64(i))
		d.refs = append(d.refs, ref)
		d.created[ref] = base.Add(time.Duration(i) * time.Hour)
	}
	d.refs = append(d.refs,
		"localhost/declarative-agents/runtime/agent-core:local",
		"kindrig/traefik:v3.7.10",
		"docker.io/library/traefik:v3.7.10",
		"ollama/ollama:0.34.2",
		"ghcr.io/nokia-bell-labs/declarative-agents/agent-core:a1b2c3d4e5f6",
	)
	return d
}

func TestCleanImagesKeepsNewestRevisionsPerFamily(t *testing.T) {
	d := newImageDaemon(t)
	if err := cleanCommitImages(d.run, 3, false); err != nil {
		t.Fatal(err)
	}
	if len(d.removed) != 7 {
		t.Fatalf("removed %d images, want 7 oldest locals: %v", len(d.removed), d.removed)
	}
	removed := strings.Join(d.removed, "\n")
	for i := 7; i < 10; i++ {
		if keep := gitLocal(t, "agent-core", 0xa00000000000+uint64(i)); strings.Contains(removed, keep) {
			t.Errorf("removed one of the newest three: %s", keep)
		}
	}
	for _, forbidden := range []string{
		":local", "kindrig/traefik", "docker.io/library/traefik", "ollama/ollama",
		"ghcr.io/nokia-bell-labs/declarative-agents/agent-core:a1b2c3d4e5f6",
	} {
		if strings.Contains(removed, forbidden) {
			t.Errorf("removed a protected or diagnosed image matching %q", forbidden)
		}
	}
}

func TestCleanImagesDryRunRemovesNothing(t *testing.T) {
	d := newImageDaemon(t)
	if err := cleanCommitImages(d.run, 3, true); err != nil {
		t.Fatal(err)
	}
	if len(d.removed) != 0 {
		t.Fatalf("dry run removed %v", d.removed)
	}
}

func TestCleanImagesKeysOnRigLabelsAndKeepsForeignSources(t *testing.T) {
	d := newImageDaemon(t)
	foreign := gitLocal(t, "agent-core", 0xa00000000000)
	rigBuilt := gitLocal(t, "agent-core", 0xa00000000001)
	sourced := gitLocal(t, "agent-core", 0xa00000000002)
	d.labels[foreign] = `{"org.opencontainers.image.source":"https://github.com/other/project"}`
	d.labels[rigBuilt] = `{"io.declarative-agents.agent-core.recipe":"sha256:x",` +
		`"org.opencontainers.image.revision":"ffffffffffff0000000000000000000000000000"}`
	d.labels[sourced] = `{"org.opencontainers.image.source":"https://github.com/Nokia-Bell-Labs/declarative-agents"}`
	if err := cleanCommitImages(d.run, 3, false); err != nil {
		t.Fatal(err)
	}
	removed := strings.Join(d.removed, "\n")
	if strings.Contains(removed, foreign) {
		t.Fatal("removed an image whose labels name another source")
	}
	for _, want := range []string{rigBuilt, sourced} {
		if !strings.Contains(removed, want) {
			t.Errorf("kept an old rig image %s", want)
		}
	}
}

func TestCleanImagesReportsInUseImagesAndContinues(t *testing.T) {
	d := newImageDaemon(t)
	busy := gitLocal(t, "agent-core", 0xa00000000000)
	d.inUse[busy] = true
	err := cleanCommitImages(d.run, 3, false)
	if err == nil || !strings.Contains(err.Error(), busy) {
		t.Fatalf("error = %v, want the in-use image named", err)
	}
	if len(d.removed) != 6 {
		t.Fatalf("removed %d, want every other candidate: %v", len(d.removed), d.removed)
	}
}

func TestCleanImagesRequiresKeepingARevision(t *testing.T) {
	if err := cleanCommitImages(newImageDaemon(t).run, 0, true); err == nil {
		t.Fatal("keep=0 accepted")
	}
}

func TestCleanImagesProtectsActiveLease(t *testing.T) {
	d := newImageDaemon(t)
	protected := gitLocal(t, "agent-core", 0xa00000000000)
	previous := commitImageLeaseStatus
	commitImageLeaseStatus = func(reference string) (bool, string) {
		return reference == protected, "test owner"
	}
	t.Cleanup(func() { commitImageLeaseStatus = previous })

	if err := cleanCommitImages(d.run, 3, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(d.removed, "\n"), protected) {
		t.Fatalf("removed actively leased image %s", protected)
	}
}

func TestCleanImagesRetainsConfiguredUpstreamPins(t *testing.T) {
	d := newImageDaemon(t)
	if !isConfiguredUpstreamPin("docker.io/library/traefik:v3.7.10") {
		t.Fatal("traefik tag-only listing was not recognized as the configured pin")
	}
	if err := cleanCommitImages(d.run, 3, false); err != nil {
		t.Fatal(err)
	}
	for _, pin := range kindrig.ConfiguredUpstreamPins() {
		if strings.Contains(strings.Join(d.removed, "\n"), pin) {
			t.Fatalf("removed configured pin %s", pin)
		}
	}
}

func TestCleanImagesDiagnosesKindrigAndRetiredTags(t *testing.T) {
	d := newImageDaemon(t)
	if err := cleanCommitImages(d.run, 3, false); err != nil {
		t.Fatal(err)
	}
	removed := strings.Join(d.removed, "\n")
	for _, leftover := range []string{
		"kindrig/traefik:v3.7.10",
		"localhost/declarative-agents/runtime/agent-core:local",
		"ghcr.io/nokia-bell-labs/declarative-agents/agent-core:a1b2c3d4e5f6",
	} {
		if strings.Contains(removed, leftover) {
			t.Errorf("swept diagnosed leftover %s", leftover)
		}
	}
}

func TestCleanImagesBoundsEachRoleSeparately(t *testing.T) {
	d := newImageDaemon(t)
	base := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ref := cacheLocal(t, "ollama-models", 0xb00000000000+uint64(i))
		d.refs = append(d.refs, ref)
		d.created[ref] = base.Add(time.Duration(i) * time.Hour)
	}
	if err := cleanCommitImages(d.run, 3, false); err != nil {
		t.Fatal(err)
	}
	removed := strings.Join(d.removed, "\n")
	var cacheRemoved int
	for i := 0; i < 5; i++ {
		ref := cacheLocal(t, "ollama-models", 0xb00000000000+uint64(i))
		if strings.Contains(removed, ref) {
			cacheRemoved++
		}
	}
	if cacheRemoved != 2 {
		t.Fatalf("cache removals = %d, want 2 oldest of 5: %v", cacheRemoved, d.removed)
	}
}

func TestCleanImagesStopsWhenReconcileFails(t *testing.T) {
	d := newImageDaemon(t)
	cleanImageReconcile = func() error { return errors.New("kind get clusters: connection refused") }
	if err := cleanCommitImages(d.run, 3, false); err == nil || !strings.Contains(err.Error(), "reconcile") {
		t.Fatalf("reconcile failure = %v", err)
	}
	if len(d.removed) != 0 {
		t.Fatalf("removed images after failed reconcile: %v", d.removed)
	}
}

func TestImageLeaseRecoverAcceptsTypedLocalAndRejectsUpstream(t *testing.T) {
	err := (CLEAN{}).ImageLeaseRecover("kindrig/traefik:v3.7.10")
	if err == nil || !strings.Contains(err.Error(), "kindrig") {
		t.Fatalf("kindrig recover = %v", err)
	}
	err = (CLEAN{}).ImageLeaseRecover(kindrig.CLIDonorImage)
	if err == nil || !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("upstream recover = %v", err)
	}
	err = (CLEAN{}).ImageLeaseRecover("ghcr.io/nokia-bell-labs/declarative-agents/agent-core:a1b2c3d4e5f6")
	if err == nil {
		t.Fatal("untyped recover accepted")
	}
}
