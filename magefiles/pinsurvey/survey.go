// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pinsurvey

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Verdict is what the survey concluded about one pin (srd006 R2, R3, R5).
type Verdict string

const (
	// Moved: the recorded digest is not what the tag resolves to now. The
	// tag was re-pushed, which is a supply-chain event rather than a
	// maintenance one, so it sorts first.
	Moved Verdict = "moved"
	// Behind: a newer upstream release exists.
	Behind Verdict = "behind"
	// Current: the digest matches and no newer release was found.
	Current Verdict = "current"
	// Unknown: the registry could not be reached or the tag is absent.
	// Never reported as current: an unreachable registry is an absence of
	// evidence, and reading it as a pass would make the report quietest
	// when it is least informed (srd006 R5.1).
	Unknown Verdict = "unknown"
)

// verdictOrder sorts a report so the findings that need a decision come
// first and the pins needing nothing come last.
var verdictOrder = map[Verdict]int{Moved: 0, Unknown: 1, Behind: 2, Current: 3}

// Pin is one pinned upstream dependency and where it is recorded.
type Pin struct {
	// Location is the file and symbol or field holding the pin, so a report
	// names the line to edit (srd006 R6.1).
	Location string
	Image    string
	Tag      string
	Digest   string
	// Architecture is set where the pin records a per-architecture digest
	// rather than the index digest: the rig pulls and retags for the host it
	// runs on, while a chart installs on either architecture.
	Architecture string
}

// Result is the survey's conclusion about one pin.
type Result struct {
	Pin      Pin
	Verdict  Verdict
	Resolved string
	Latest   string
	Reason   string
}

func (r Result) String() string {
	switch r.Verdict {
	case Moved:
		return fmt.Sprintf("%s: %s:%s records %s but the tag resolves to %s",
			r.Pin.Location, r.Pin.Image, r.Pin.Tag, r.Pin.Digest, r.Resolved)
	case Behind:
		return fmt.Sprintf("%s: %s is pinned at %s, upstream has %s",
			r.Pin.Location, r.Pin.Image, r.Pin.Tag, r.Latest)
	case Unknown:
		return fmt.Sprintf("%s: %s:%s could not be checked (%s)",
			r.Pin.Location, r.Pin.Image, r.Pin.Tag, r.Reason)
	default:
		return fmt.Sprintf("%s: %s:%s", r.Pin.Location, r.Pin.Image, r.Pin.Tag)
	}
}

// Resolver is what the survey needs from a registry, so a test supplies its
// own without an HTTP server when the case is about verdicts rather than
// about the protocol.
type Resolver interface {
	ResolveDigest(Reference) (string, error)
	ListTags(Reference) ([]string, error)
}

// Survey resolves every pin and returns one result each, ordered by verdict
// and then by location so two runs over one inventory report identically.
func Survey(resolver Resolver, pins []Pin) []Result {
	results := make([]Result, 0, len(pins))
	for _, pin := range pins {
		results = append(results, surveyOne(resolver, pin))
	}
	sort.SliceStable(results, func(i, j int) bool {
		if verdictOrder[results[i].Verdict] != verdictOrder[results[j].Verdict] {
			return verdictOrder[results[i].Verdict] < verdictOrder[results[j].Verdict]
		}
		return results[i].Pin.Location < results[j].Pin.Location
	})
	return results
}

func surveyOne(resolver Resolver, pin Pin) Result {
	reference, err := ParseReference(pin.Image)
	if err != nil {
		return Result{Pin: pin, Verdict: Unknown, Reason: err.Error()}
	}
	reference.Tag = pin.Tag
	reference.Architecture = pin.Architecture
	resolved, err := resolver.ResolveDigest(reference)
	if err != nil {
		return Result{Pin: pin, Verdict: Unknown, Reason: err.Error()}
	}
	// Integrity outranks currency: a tag that moved under a pin is a
	// different problem from one that merely aged, and reporting the
	// staleness of an image nobody chose would bury it.
	if pin.Digest != "" && resolved != pin.Digest {
		return Result{Pin: pin, Verdict: Moved, Resolved: resolved}
	}
	pinned, ok := parseVersion(pin.Tag)
	if !ok {
		// A commit revision or a fail-closed sentinel has no upstream
		// successor to compare against; integrity is the whole answer.
		return Result{Pin: pin, Verdict: Current, Resolved: resolved}
	}
	tags, err := resolver.ListTags(reference)
	if err != nil {
		return Result{Pin: pin, Verdict: Unknown, Resolved: resolved, Reason: err.Error()}
	}
	latest, latestTag := pinned, ""
	for _, tag := range tags {
		candidate, ok := parseVersion(tag)
		if !ok || candidate.prerelease {
			continue
		}
		switch {
		case candidate.after(latest):
			latest, latestTag = candidate, tag
		case latestTag != "" && !candidate.after(latest) && !latest.after(candidate):
			// Same version, different spelling. Prefer the one written the
			// way the pin is, so the reported tag is the one to paste.
			if sameVersionStyle(pin.Tag, tag) && !sameVersionStyle(pin.Tag, latestTag) {
				latestTag = tag
			}
		}
	}
	if latestTag != "" {
		return Result{Pin: pin, Verdict: Behind, Resolved: resolved, Latest: latestTag}
	}
	return Result{Pin: pin, Verdict: Current, Resolved: resolved}
}

// version is a semantic version read out of a tag.
type version struct {
	parts      [3]int
	prerelease bool
}

// semverTag matches a tag that names a version, with or without the leading
// v the container ecosystem uses inconsistently. A trailing suffix marks a
// pre-release, which never counts as newer than a release.
var semverTag = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?(.*)$`)

func parseVersion(tag string) (version, bool) {
	match := semverTag.FindStringSubmatch(strings.TrimSpace(tag))
	if match == nil {
		return version{}, false
	}
	var parsed version
	for index, group := range match[1:4] {
		if group == "" {
			continue
		}
		value, err := strconv.Atoi(group)
		if err != nil {
			return version{}, false
		}
		parsed.parts[index] = value
	}
	if suffix := match[4]; suffix != "" {
		// A variant tag such as 0.34.2-rocm names a different image rather
		// than a later one, and a release candidate is not a release.
		parsed.prerelease = true
	}
	return parsed, true
}

func (v version) after(other version) bool {
	for index := range v.parts {
		if v.parts[index] != other.parts[index] {
			return v.parts[index] > other.parts[index]
		}
	}
	return false
}

// Counts summarizes a survey for the report's closing line.
func Counts(results []Result) map[Verdict]int {
	counts := map[Verdict]int{}
	for _, result := range results {
		counts[result.Verdict]++
	}
	return counts
}

// sameVersionStyle reports whether two tags agree on the leading v, which
// the container ecosystem uses inconsistently and often publishes both ways.
func sameVersionStyle(a, b string) bool {
	return strings.HasPrefix(a, "v") == strings.HasPrefix(b, "v")
}
