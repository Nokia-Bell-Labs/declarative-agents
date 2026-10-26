// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// Candidate is one version scrum-master release next offers.
type Candidate struct {
	Bump    string `json:"bump"`
	Version string `json:"version"`
	Tag     string `json:"tag"`
	Class   string `json:"class"`
	// Label is Class as text says it, naming a pre-1.0 minor's break.
	Label string `json:"-"`
}

// NextResult is what scrum-master release next reports (srd015 R3.1).
type NextResult struct {
	Artifact   string      `json:"artifact"`
	Latest     string      `json:"latest,omitempty"`
	LatestTag  string      `json:"latest_tag,omitempty"`
	Candidates []Candidate `json:"candidates"`
	// Recommended is the candidate the commits since the latest tag call
	// for, and Reason says why; empty when nothing changed since it.
	Recommended string `json:"recommended,omitempty"`
	// RecommendedVersion is the recommended candidate's version, which the
	// release profile's gate and tag read (GH-37).
	RecommendedVersion string `json:"recommended_version,omitempty"`
	Reason             string `json:"reason"`
	// Chosen is the candidate version a person chose over the
	// recommendation; the release profile's gate and tag then read it as
	// recommended_version (GH-47).
	Chosen string `json:"chosen,omitempty"`
}

// Choose makes version, one of the candidates, the version the release
// gates and tags in place of the recommendation, and says so in Reason.
func (n *NextResult) Choose(version string) error {
	for _, c := range n.Candidates {
		if c.Version != version {
			continue
		}
		recommended := n.RecommendedVersion
		n.Chosen, n.RecommendedVersion = version, version
		if recommended == "" {
			n.Reason += "; chosen: " + c.Bump + " " + version
		} else if recommended != version {
			n.Reason += "; chosen: " + c.Bump + " " + version + " over the recommended " + recommended
		}
		return nil
	}
	versions := make([]string, 0, len(n.Candidates))
	for _, c := range n.Candidates {
		versions = append(versions, c.Version)
	}
	return fmt.Errorf("%s is not a candidate for %s; choose one of %s", version, n.Artifact, strings.Join(versions, ", "))
}

// Recommend picks the bump the commits since the latest release call for
// (srd015 R3.1): a first release is minor; a commit that says BREAKING
// CHANGE makes it major; commits that all fix something, or that change no
// requirement of a corpus (requirementsUnchanged), a republish, make it a
// patch; any other change is minor. No commit since the latest release
// recommends nothing.
func (n *NextResult) Recommend(messages []string, requirementsUnchanged bool) {
	defer func() {
		n.RecommendedVersion = ""
		for _, c := range n.Candidates {
			if c.Bump == n.Recommended {
				n.RecommendedVersion = c.Version
			}
		}
	}()
	switch {
	case n.Latest == "":
		n.Recommended, n.Reason = "minor", "no "+n.Artifact+" tag yet: the first release is minor"
		return
	case len(messages) == 0:
		n.Recommended, n.Reason = "", "no commit since "+n.LatestTag+": nothing to release"
		return
	}
	fixes := 0
	for _, m := range messages {
		if strings.Contains(m, "BREAKING CHANGE") {
			n.Recommended, n.Reason = "major", "a commit since "+n.LatestTag+" says BREAKING CHANGE"
			return
		}
		if subject := strings.ToLower(strings.TrimSpace(m)); strings.HasPrefix(subject, "fix") {
			fixes++
		}
	}
	if fixes == len(messages) {
		n.Recommended, n.Reason = "patch", fmt.Sprintf("all %d commits since %s fix something", fixes, n.LatestTag)
		return
	}
	if requirementsUnchanged {
		n.Recommended, n.Reason = "patch", fmt.Sprintf("%d commits since %s change no requirement: a republish", len(messages), n.LatestTag)
		return
	}
	n.Recommended, n.Reason = "minor", fmt.Sprintf("%d commits since %s change more than fixes", len(messages), n.LatestTag)
}

// Next reports the artifact's latest version among tags and the next
// patch, minor, and major versions with their class. An artifact with no
// tag starts from 0.0.0.
func Next(r program.Releases, artifact string, tags []string) NextResult {
	out := NextResult{Artifact: artifact}
	latest, ok := Latest(r.TagFormat, artifact, tags)
	if ok {
		out.Latest, out.LatestTag = latest.String(), TagName(r.TagFormat, artifact, latest)
	}
	for _, bump := range []string{"patch", "minor", "major"} {
		v := latest.Bump(bump)
		out.Candidates = append(out.Candidates, Candidate{bump, v.String(), TagName(r.TagFormat, artifact, v), Class(r.Breaking, latest, v), ClassLabel(r.Breaking, latest, v)})
	}
	return out
}

// Human is the text scrum-master release next prints without --json.
func (n NextResult) Human() string {
	var b strings.Builder
	if n.Latest == "" {
		b.WriteString(n.Artifact + ": no release yet\n")
	} else {
		b.WriteString(n.Artifact + ": latest " + n.Latest + " (" + n.LatestTag + ")\n")
	}
	for _, c := range n.Candidates {
		mark := ""
		if c.Bump == n.Recommended {
			mark = " (recommended)"
		}
		b.WriteString("  " + c.Bump + " " + c.Version + " " + c.Label + mark + "\n")
	}
	if n.Reason != "" {
		b.WriteString("  " + n.Reason + "\n")
	}
	return b.String()
}
