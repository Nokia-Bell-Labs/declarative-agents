// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeImageDaemon serves docker image ls/inspect/rm for scripted tags and
// records removals. labels maps a ref to its image labels as JSON (default null).
type fakeImageDaemon struct {
	tags    map[string][]string
	created map[string]time.Time
	labels  map[string]string
	inUse   map[string]bool
	removed []string
}

func (d *fakeImageDaemon) run(args ...string) ([]byte, error) {
	switch {
	case len(args) == 5 && args[0] == "image" && args[1] == "ls":
		return []byte(strings.Join(d.tags[args[4]], "\n") + "\n"), nil
	case len(args) == 5 && args[0] == "image" && args[1] == "inspect":
		ref := args[4]
		labels := d.labels[ref]
		if labels == "" {
			labels = "null"
		}
		return []byte(d.created[ref].Format(time.RFC3339Nano) + "|" + labels + "\n"), nil
	case len(args) == 3 && args[0] == "image" && args[1] == "rm":
		if d.inUse[args[2]] {
			return []byte("image is being used by running container"), errors.New("conflict")
		}
		d.removed = append(d.removed, args[2])
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected docker %v", args)
}

// newImageDaemon gives every family ten commit revisions (r0 oldest) plus a
// :local and a :latest tag, and adds an unrelated third-party repository.
func newImageDaemon() *fakeImageDaemon {
	d := &fakeImageDaemon{
		tags:    map[string][]string{},
		created: map[string]time.Time{},
		labels:  map[string]string{},
		inUse:   map[string]bool{},
	}
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, family := range commitImageFamilies {
		for i := 0; i < 10; i++ {
			tag := fmt.Sprintf("%012x", 0xa00000000000+i)
			ref := family + ":" + tag
			d.tags[family] = append(d.tags[family], tag)
			d.created[ref] = base.Add(time.Duration(i) * time.Hour)
		}
		d.tags[family] = append(d.tags[family], "local", "latest")
	}
	d.tags["ollama/ollama"] = []string{"0a0000000001"}
	return d
}

func TestCleanImagesKeepsNewestRevisionsPerFamily(t *testing.T) {
	d := newImageDaemon()
	if err := cleanCommitImages(d.run, 3, false); err != nil {
		t.Fatal(err)
	}
	if len(d.removed) != 7*len(commitImageFamilies) {
		t.Fatalf("removed %d images, want 7 per family: %v", len(d.removed), d.removed)
	}
	removed := strings.Join(d.removed, "\n")
	for _, family := range commitImageFamilies {
		for i := 7; i < 10; i++ {
			if keep := fmt.Sprintf("%s:%012x", family, 0xa00000000000+i); strings.Contains(removed, keep) {
				t.Errorf("removed one of the newest three: %s", keep)
			}
		}
	}
	for _, forbidden := range []string{":local", ":latest", "ollama/ollama"} {
		if strings.Contains(removed, forbidden) {
			t.Errorf("removed a protected image matching %q", forbidden)
		}
	}
}

func TestCleanImagesDryRunRemovesNothing(t *testing.T) {
	d := newImageDaemon()
	if err := cleanCommitImages(d.run, 3, true); err != nil {
		t.Fatal(err)
	}
	if len(d.removed) != 0 {
		t.Fatalf("dry run removed %v", d.removed)
	}
}

func TestCleanImagesKeysOnRigLabelsAndKeepsForeignSources(t *testing.T) {
	d := newImageDaemon()
	family := commitImageFamilies[0]
	foreign := fmt.Sprintf("%s:%012x", family, 0xa00000000000)
	rigBuilt := fmt.Sprintf("%s:%012x", family, 0xa00000000001)
	sourced := fmt.Sprintf("%s:%012x", family, 0xa00000000002)
	d.labels[foreign] = `{"org.opencontainers.image.source":"https://github.com/other/project"}`
	// The revision label records the agent-core source revision, which need
	// not match the tag: a reused image is retagged for each commit.
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
	d := newImageDaemon()
	busy := fmt.Sprintf("%s:%012x", commitImageFamilies[0], 0xa00000000000)
	d.inUse[busy] = true
	err := cleanCommitImages(d.run, 3, false)
	if err == nil || !strings.Contains(err.Error(), busy) {
		t.Fatalf("error = %v, want the in-use image named", err)
	}
	if len(d.removed) != 7*len(commitImageFamilies)-1 {
		t.Fatalf("removed %d, want every other candidate", len(d.removed))
	}
}

func TestCleanImagesRequiresKeepingARevision(t *testing.T) {
	if err := cleanCommitImages(newImageDaemon().run, 0, true); err == nil {
		t.Fatal("keep=0 accepted")
	}
}
