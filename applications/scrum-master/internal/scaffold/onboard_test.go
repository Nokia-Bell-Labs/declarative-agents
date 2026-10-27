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

// scrumJoining is the Platform scrum's planning repository joining a
// program whose system has no SRD for it yet.
func scrumJoining(dir string) OnboardOptions {
	o := joining(dir)
	o.Subsystem, o.Repository, o.Role, o.Owner = "srd002-platform", "https://git.example.invalid/g/platform/tpm", RoleScrum, "Product Owner, platform"
	return o
}

// A scrum repository joining a subsystem with no SRD files one issue that
// creates the SRD, naming the repository as its scrum repository and
// declaring its component there until a code repository takes it, and a
// second run files nothing (srd043 R2; GH-2745).
func TestOnboardAScrumRepositoryCreatesItsSRD(t *testing.T) {
	f := system(t)
	f.AddFile("g/system", "main", systemSRDPath, []byte(strings.Replace(subsystemSRD, "srd010-ingest", "srd001-system", -1)+
		"| system | https://git.example.invalid/g/system | / |\n"))
	r, err := Onboard(f, scrumJoining(repo(t)))
	if err != nil || !r.Registration.Filed {
		t.Fatalf("registration: %+v, %v", r.Registration, err)
	}
	issues, _ := f.ListIssues("g/system", tracker.IssueQuery{State: "opened"})
	if len(issues) != 1 {
		t.Fatalf("%d issues, want 1", len(issues))
	}
	i := issues[0]
	for _, want := range []string{"Create srd002-platform with g/platform/tpm", "id: srd002-platform", "- owner: Product Owner, platform",
		"- scrum_repository: https://git.example.invalid/g/platform/tpm", "| platform | https://git.example.invalid/g/platform/tpm | / |"} {
		if !strings.Contains(i.Title+"\n"+i.Description, want) {
			t.Errorf("the registration lacks %q:\n%s\n%s", want, i.Title, i.Description)
		}
	}
	if strings.Contains(i.Description, "add the row") || slices.Contains(i.Labels, "needs-triage") {
		t.Errorf("the registration edits a missing SRD or failed admission: %+v", i)
	}
	again, err := Onboard(f, scrumJoining(repo(t)))
	if err != nil || again.Registration.Filed || !strings.Contains(again.Registration.Reason, "already asks") {
		t.Fatalf("second onboard: %+v, %v", again.Registration, err)
	}
}

// A scrum repository already named as a subsystem's scrum_repository files
// nothing and names the SRD; one joining an existing SRD asks to set its
// scrum_repository, not to add a component row (GH-2745).
func TestOnboardAScrumRepositoryOfAnExistingSRD(t *testing.T) {
	f := system(t)
	f.AddFile("g/system", "main", "docs/specs/software-requirements/srd002-platform.md", []byte(strings.NewReplacer("srd010-ingest", "srd002-platform",
		"- owner: Product Owner, ingest", "- owner: Product Owner, platform\n- scrum_repository: https://git.example.invalid/g/platform/tpm",
		"| ingest | https://git.example.invalid/g/ingest | / |", "| platform | https://git.example.invalid/g/platform/tpm | / |").Replace(subsystemSRD)))
	r, err := Onboard(f, scrumJoining(repo(t)))
	if err != nil || r.Registration.Filed || !strings.Contains(r.Registration.Reason, "srd002-platform already names g/platform/tpm as its scrum repository") {
		t.Fatalf("a named scrum repository: %+v, %v", r.Registration, err)
	}
	o := scrumJoining(repo(t))
	o.Subsystem = "srd010-ingest"
	o.Repository = "https://git.example.invalid/g/ingest-scrum"
	if r, err = Onboard(f, o); err != nil || !r.Registration.Filed {
		t.Fatalf("a scrum repository for ingest: %+v, %v", r.Registration, err)
	}
	issues, _ := f.ListIssues("g/system", tracker.IssueQuery{State: "opened"})
	if len(issues) != 1 || !strings.Contains(issues[0].Description, "add the context item scrum_repository: https://git.example.invalid/g/ingest-scrum") || strings.Contains(issues[0].Description, "add the row") {
		t.Fatalf("registration for an existing SRD: %+v", issues)
	}
}
