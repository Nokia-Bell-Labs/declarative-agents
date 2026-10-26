// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

func TestNextReportsLatestAndCandidates(t *testing.T) {
	r := program.Defaults().Releases
	n := Next(r, "a", []string{"a/v0.1.0", "a/v0.2.3", "b/v9.0.0", "a/v0.2.3-rc1", "v0.20261015.2"})
	if n.Latest != "0.2.3" || n.LatestTag != "a/v0.2.3" {
		t.Fatalf("latest %+v", n)
	}
	want := []Candidate{
		{"patch", "0.2.4", "a/v0.2.4", "compatible", "compatible"},
		{"minor", "0.3.0", "a/v0.3.0", "breaking", "breaking (minor before 1.0)"},
		{"major", "1.0.0", "a/v1.0.0", "breaking", "breaking"},
	}
	for i, c := range want {
		if n.Candidates[i] != c {
			t.Fatalf("candidate %d: %+v, want %+v", i, n.Candidates[i], c)
		}
	}

	n = Next(r, "fresh", nil)
	if n.Latest != "" || n.Candidates[0].Version != "0.0.1" || n.Candidates[1].Version != "0.1.0" || n.Candidates[2].Version != "1.0.0" {
		t.Fatalf("no tag: %+v", n)
	}
}

func TestBreakingClassFollowsPolicy(t *testing.T) {
	v := func(s string) Version { x, _ := ParseVersion(s); return x }
	for _, c := range []struct {
		rule, from, to string
		breaking       bool
	}{
		{"major-and-minor-before-1.0", "0.2.3", "0.3.0", true},
		{"major-and-minor-before-1.0", "0.2.3", "0.2.4", false},
		{"major-and-minor-before-1.0", "1.2.3", "1.3.0", false},
		{"major", "0.2.3", "0.3.0", false},
		{"major", "1.2.3", "2.0.0", true},
	} {
		if got := Breaking(c.rule, v(c.from), v(c.to)); got != c.breaking {
			t.Errorf("%s %s -> %s: breaking %v", c.rule, c.from, c.to, got)
		}
	}
}

func TestPolicyLoadsReleasesAndDefinitionOfDone(t *testing.T) {
	f := fake.New()
	f.AddFile("g/system", "main", "control/policy.md", []byte(`---
id: policy
title: Policy
---

<!-- field: version -->
## Version

1

<!-- field: releases -->
## Releases

<!-- field: scheme -->
### Scheme

semver

<!-- field: breaking -->
### Breaking

major

<!-- field: tag_format -->
### Tag format

v{version}

<!-- field: definition_of_done -->
## Definition of done

<!-- field: corpus -->
### Corpus

| kind | environment | command |
| --- | --- | --- |
| custom | local | mage audit |

<!-- field: agent-profile -->
### Agent-profile

| kind | environment | against | threshold |
| --- | --- | --- | --- |
| scorecard | dev | current | no-regression |
| peer-read |  |  |  |
`))
	p, err := program.LoadPolicy(f, "g/system", "main")
	if err != nil {
		t.Fatal(err)
	}
	if p.Releases.Breaking != "major" || p.Releases.TagFormat != "v{version}" {
		t.Fatalf("releases %+v", p.Releases)
	}
	if g := p.Gate("corpus"); len(g) != 1 || g[0].Command != "mage audit" {
		t.Fatalf("corpus gate %+v", g)
	}
	if g := p.Gate("agent-profile"); len(g) != 2 || g[1].Kind != "peer-read" || g[0].Threshold != "no-regression" {
		t.Fatalf("agent-profile gate %+v", g)
	}
	if g := p.Gate("other"); g != nil {
		t.Fatalf("an artifact with no entry and no default gets %+v", g)
	}
	if g := program.Defaults().Gate("anything"); len(g) != 1 || g[0].Command != "go test ./..." {
		t.Fatalf("default gate %+v", g)
	}
}

// A chosen candidate replaces the recommendation the gate and tag read,
// and says so; a version that is no candidate is refused (GH-47).
func TestNextChoosesACandidate(t *testing.T) {
	n := Next(program.Defaults().Releases, "a", []string{"a/v0.2.3"})
	n.Recommend([]string{"Add the scorer"}, false)
	if n.RecommendedVersion != "0.3.0" {
		t.Fatalf("recommended %q, want the minor 0.3.0", n.RecommendedVersion)
	}
	if err := n.Choose("0.2.4"); err != nil {
		t.Fatal(err)
	}
	if n.RecommendedVersion != "0.2.4" || n.Chosen != "0.2.4" || !strings.Contains(n.Reason, "chosen: patch 0.2.4 over the recommended 0.3.0") {
		t.Fatalf("after choosing the patch: %+v", n)
	}
	if err := n.Choose("0.9.9"); err == nil || !strings.Contains(err.Error(), "not a candidate") {
		t.Fatalf("a version no candidate names = %v, want a refusal", err)
	}
}
