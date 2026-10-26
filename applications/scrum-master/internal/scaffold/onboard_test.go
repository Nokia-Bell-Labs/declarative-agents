// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	scrummaster "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// subsystemSRD lists one component, the repository the program already has.
const subsystemSRD = `---
id: srd010-ingest
title: "Subsystem: ingest"
---

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, ingest

<!-- req: R1 -->
## R1 - Ingest

<!-- req: R1.1 -->
### R1.1

Records arrive.

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| ingest | https://git.example.invalid/g/ingest | / |
`

func system(t *testing.T) *fake.Fake {
	t.Helper()
	f := fake.New()
	f.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-ingest.md", []byte(subsystemSRD))
	return f
}

// TestRegistrationCarriesTheSystemComponent files the registration with the
// system repository's component once the system SRD declares it.
func TestRegistrationCarriesTheSystemComponent(t *testing.T) {
	f := system(t)
	f.AddFile("g/system", "main", systemSRDPath, []byte(strings.Replace(subsystemSRD, "srd010-ingest", "srd001-system", -1)+
		"| system | https://git.example.invalid/g/system | / |\n"))
	if _, err := Onboard(f, joining(repo(t))); err != nil {
		t.Fatal(err)
	}
	issues, _ := f.ListIssues("g/system", tracker.IssueQuery{State: "opened"})
	if len(issues) != 1 || !slices.Contains(issues[0].Labels, "component::system") || slices.Contains(issues[0].Labels, "needs-triage") {
		t.Fatalf("registration issue: %+v", issues)
	}
}

func joining(dir string) OnboardOptions {
	return OnboardOptions{Dir: dir, System: "g/system", Ref: "main", Subsystem: "srd010-ingest", Repository: "https://git.example.invalid/g/store"}
}

func TestOnboardWritesMemberSkeleton(t *testing.T) {
	dir := repo(t)
	r, err := Onboard(system(t), joining(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := actions(r.Files)
	if got["docs/specs/software-requirements/srd001-subsystem.md"] != "created" || got["docs/VISION.md"] != "created" {
		t.Fatalf("member skeleton: %v", got)
	}
	for p := range got {
		if strings.HasPrefix(p, "control/") || strings.Contains(p, "srd001-system") {
			t.Errorf("a member repository got %s", p)
		}
	}
	vision, _ := os.ReadFile(filepath.Join(dir, "docs", "VISION.md"))
	if !strings.Contains(string(vision), "The Product Owner writes here") || strings.Contains(string(vision), "{{") {
		t.Fatalf("member VISION:\n%s", vision)
	}
}

func TestOnboardWritesOrientation(t *testing.T) {
	dir := repo(t)
	r, err := Onboard(system(t), joining(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := actions(r.Files)
	for f, want := range map[string]string{
		"README.md":                     "# Team subsystem repository",
		"docs/specs/epics/README.md":    "`epNNN-short-name.md`",
		"docs/specs/features/README.md": "`ftNNN-short-name.md`",
		"docs/specs/stories/README.md":  "`stNNN-short-name.md`",
		"docs/engineering/README.md":    "`engNN-short-name.md`",
	} {
		b, _ := os.ReadFile(filepath.Join(dir, f))
		if got[f] != "created" || !strings.Contains(string(b), want) || !strings.Contains(string(b), orientationMarkers[0]) {
			t.Errorf("%s: %q\n%s", f, got[f], b)
		}
	}
	for _, f := range []string{"docs/specs/themes/README.md", "docs/specs/initiatives/README.md"} {
		if _, ok := got[f]; ok {
			t.Errorf("a member repository got %s", f)
		}
	}
	// A team's AGENTS.md names every skill onboarding installs, tasks among
	// them: Sprint Planning hands off to it (#188).
	agents, _ := os.ReadFile(filepath.Join(dir, AgentsFile))
	skills, err := install.Skills(scrummaster.Plugin, scrummaster.SkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if !strings.Contains(string(agents), "- "+s.Name+": ") {
			t.Errorf("the subsystem AGENTS.md does not name the %s skill", s.Name)
		}
	}
}

func TestOnboardAppendsToExistingAgentsAndReadme(t *testing.T) {
	dir := repo(t)
	own := map[string]string{"README.md": "# store\n\nThe team's own README.\n", AgentsFile: "Use tabs.\n"}
	for f, text := range own {
		os.WriteFile(filepath.Join(dir, f), []byte(text), 0o644)
	}
	r, err := Onboard(system(t), joining(dir))
	if err != nil {
		t.Fatal(err)
	}
	for f, text := range own {
		b, _ := os.ReadFile(filepath.Join(dir, f))
		if actions(r.Files)[f] != "appended" || !strings.HasPrefix(string(b), text+"\n"+BlockBegin+"\n") || !strings.HasSuffix(string(b), BlockEnd+"\n") {
			t.Errorf("%s: %s\n%s", f, actions(r.Files)[f], b)
		}
	}
}

func TestOnboardFilesOneRegistration(t *testing.T) {
	f := system(t)
	before := len(f.Files)
	r, err := Onboard(f, joining(repo(t)))
	if err != nil || !r.Registration.Filed || r.Registration.Issue == nil || r.Registration.Issue.Plane != "management" {
		t.Fatalf("registration: %+v, %v", r.Registration, err)
	}
	issues, _ := f.ListIssues("g/system", tracker.IssueQuery{State: "opened"})
	if len(issues) != 1 {
		t.Fatalf("%d issues in the system repository, want 1", len(issues))
	}
	i := issues[0]
	if !slices.Contains(i.Labels, "plane::management") || !slices.Contains(i.Labels, "op::onboard") || !strings.Contains(i.Title, "g/store") ||
		!strings.Contains(i.Description, "(test: mage audit; environment: local)") ||
		!strings.Contains(i.Description, "srd010-ingest") || !strings.Contains(i.Description, registrationMarker("g/store")) || len(i.Assignees) != 0 {
		t.Fatalf("registration issue: %+v", i)
	}
	if len(f.Files) != before {
		t.Fatalf("onboard wrote to the system repository: %d files, was %d", len(f.Files), before)
	}
}

func TestOnboardFilesNoSecondRegistration(t *testing.T) {
	f := system(t)
	if _, err := Onboard(f, joining(repo(t))); err != nil {
		t.Fatal(err)
	}
	again, err := Onboard(f, joining(repo(t)))
	if err != nil || again.Registration.Filed || !strings.Contains(again.Registration.Reason, "already asks") {
		t.Fatalf("second onboard: %+v, %v", again.Registration, err)
	}
	listed := joining(repo(t))
	listed.Repository = "https://git.example.invalid/g/ingest"
	r, err := Onboard(f, listed)
	if err != nil || r.Registration.Filed || !strings.Contains(r.Registration.Reason, "already lists") {
		t.Fatalf("onboard of a listed repository: %+v, %v", r.Registration, err)
	}
	if issues, _ := f.ListIssues("g/system", tracker.IssueQuery{State: "opened"}); len(issues) != 1 {
		t.Fatalf("%d registrations, want 1", len(issues))
	}
}
