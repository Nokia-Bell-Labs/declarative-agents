// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

const systemSRD = `---
id: srd001-system
title: "Subsystem: system"
---

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| tpm | https://gitlab.example.com/an-gc-group/tpm | / |
`

// The one-component rule gives one verdict for an issue of the Platform
// scrum's repository whether the system's SRDs are read from its checkout
// on disk or through the tracker from the scrum's checkout, where the
// system's checkout mirrors the forge's group and shares the scrum's last
// segment, tpm: the scrum's own checkout is never read for the system's
// (GH-2744).
func TestOneComponentReadsTheSameSRDsEitherWay(t *testing.T) {
	home := t.TempDir()
	if h, err := filepath.EvalSymlinks(home); err == nil {
		home = h
	}
	t.Setenv("HOME", home)
	system := filepath.Join(home, "GITLABS", "an-gc-group", "tpm")
	scrum := filepath.Join(home, "GITLABS", "an-gc-group", "platform", "tpm")
	checkoutWith(t, system, map[string]string{checkout.TrackersFile: "bd\n", filepath.Join(SRDDir, "srd001-system.md"): systemSRD})
	beadstest.Git(t, system, "remote", "add", "origin", "https://gitlab.example.com/an-gc-group/tpm.git")
	checkoutWith(t, scrum, nil)
	beadstest.Git(t, scrum, "remote", "add", "origin", "https://gitlab.example.com/an-gc-group/platform/tpm.git")

	onDisk := Defaults()
	subs, err := SubsystemsIn(system)
	if err != nil {
		t.Fatal(err)
	}
	onDisk.Components, onDisk.ComponentsRead = subs.Components(), true
	through, err := LoadPolicy(codeFiles{fake.New(), &checkout.Code{Dir: scrum}}, "an-gc-group/tpm", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(through.Components, onDisk.Components) || !slices.Equal(onDisk.Components, []string{"tpm"}) {
		t.Fatalf("components through the tracker %v, on disk %v", through.Components, onDisk.Components)
	}
	issue := tracker.Issue{Project: "an-gc-group/platform/tpm", IID: 7, Labels: []string{"plane::data", "kind::build"}}
	want := "needs exactly one component:: label, has 0"
	for name, p := range map[string]Policy{"on disk": onDisk, "through the tracker": through} {
		if got := oneComponent(issue, p); !slices.Contains(got, want) {
			t.Errorf("%s: oneComponent = %v, want %q", name, got, want)
		}
		if note := BootstrapNote(issue, p); note != "" {
			t.Errorf("%s: the waiver applied: %s", name, note)
		}
	}
	if note := BootstrapNote(issue, Policy{ComponentsRead: true}); !strings.Contains(note, "waived") {
		t.Errorf("a program with no component says nothing of the waiver: %q", note)
	}
}

// failingDir is a tracker that cannot list the SRDs' folder.
type failingDir struct{ *fake.Fake }

func (failingDir) ListDir(string, string, string) ([]string, error) {
	return nil, errors.New("403 Forbidden")
}

// A tracker that cannot read the SRDs fails LoadPolicy, naming the project
// and the folder, never reads as a program with no component (GH-2744).
func TestLoadPolicyFailsOnUnreadableSRDs(t *testing.T) {
	_, err := LoadPolicy(failingDir{fake.New()}, "an-gc-group/tpm", "main")
	if err == nil || !strings.Contains(err.Error(), "an-gc-group/tpm "+SRDDir) {
		t.Fatalf("LoadPolicy = %v, want an error naming an-gc-group/tpm %s", err, SRDDir)
	}
}

const platformSRD = `---
id: srd002-platform
title: "Platform"
---

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, platform
- scrum_repository: https://gitlab.example.com/an-gc-group/platform/tpm

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| platform | https://gitlab.example.com/an-gc-group/platform/tpm | / |
`

// A team with a scrum repository and no code repository yet declares its
// component on the scrum repository until a code repository takes it: the
// SRD reads the same from disk and through the tracker, and the scrum
// repository's issues carry the component (srd043 R2; GH-2745).
func TestAScrumWithNoCodeRepositoryYetDeclaresItsComponents(t *testing.T) {
	dir := t.TempDir()
	checkoutWith(t, dir, map[string]string{filepath.Join(SRDDir, "srd002-platform.md"): platformSRD})
	onDisk, err := SubsystemsIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	f.AddFile("an-gc-group/tpm", "main", SRDDir+"/srd002-platform.md", []byte(platformSRD))
	through, err := LoadSubsystems(f, "an-gc-group/tpm", "main")
	if err != nil {
		t.Fatal(err)
	}
	for name, subs := range map[string]Subsystems{"on disk": onDisk, "through the tracker": through} {
		c, sub, ok := subs.ComponentFor("an-gc-group/platform/tpm")
		if !ok || sub.ID != "srd002-platform" || c.Name != "platform" || sub.ScrumRepository() != "an-gc-group/platform/tpm" {
			t.Errorf("%s: %+v %+v %v", name, c, sub, ok)
		}
		if got := Labels("data", "build", "", c.Name); !slices.Contains(got, "component::platform") {
			t.Errorf("%s: the scrum repository's issue labels %v", name, got)
		}
	}
}
