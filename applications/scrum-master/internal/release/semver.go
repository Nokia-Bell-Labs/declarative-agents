// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a semantic version major.minor.patch (policy.releases.scheme
// semver).
type Version struct{ Major, Minor, Patch int }

var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

// ParseVersion reads major.minor.patch, without a leading v.
func ParseVersion(s string) (Version, bool) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	return Version{a, b, c}, true
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Less orders versions numerically.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	return v.Patch < w.Patch
}

// Bump is the next patch, minor, or major version.
func (v Version) Bump(part string) Version {
	switch part {
	case "major":
		return Version{v.Major + 1, 0, 0}
	case "minor":
		return Version{v.Major, v.Minor + 1, 0}
	default:
		return Version{v.Major, v.Minor, v.Patch + 1}
	}
}

// Breaking reports whether going from one version to another is breaking
// under policy.releases.breaking: major, or major-and-minor-before-1.0,
// where a minor bump before 1.0 also breaks.
func Breaking(rule string, from, to Version) bool {
	if to.Major > from.Major {
		return true
	}
	return rule == "major-and-minor-before-1.0" && from.Major == 0 && to.Major == 0 && to.Minor > from.Minor
}

// Class names Breaking's answer as the tag message and release issue do.
func Class(rule string, from, to Version) string {
	if Breaking(rule, from, to) {
		return "breaking"
	}
	return "compatible"
}

// ClassLabel is Class as a tag message, note, or command output says it: a
// break that is only a minor bump before 1.0 says so, since the policy, not
// the change, makes it breaking.
func ClassLabel(rule string, from, to Version) string {
	if Breaking(rule, from, to) && to.Major == from.Major {
		return "breaking (minor before 1.0)"
	}
	return Class(rule, from, to)
}

// TagName is the tag for an artifact's version in tagFormat.
func TagName(tagFormat, artifact string, v Version) string {
	return strings.NewReplacer("{artifact}", artifact, "{version}", v.String()).Replace(tagFormat)
}

// Versions are the artifact's versions among tags, read through tagFormat.
func Versions(tagFormat, artifact string, tags []string) []Version {
	pattern := regexp.QuoteMeta(tagFormat)
	pattern = strings.Replace(pattern, regexp.QuoteMeta("{artifact}"), regexp.QuoteMeta(artifact), 1)
	pattern = strings.Replace(pattern, regexp.QuoteMeta("{version}"), `(\d+\.\d+\.\d+)`, 1)
	re := regexp.MustCompile("^" + pattern + "$")
	var out []Version
	for _, t := range tags {
		if m := re.FindStringSubmatch(strings.TrimSpace(t)); m != nil {
			if v, ok := ParseVersion(m[1]); ok {
				out = append(out, v)
			}
		}
	}
	return out
}

// Latest is the highest version among tags, and whether there is one.
func Latest(tagFormat, artifact string, tags []string) (Version, bool) {
	var latest Version
	found := false
	for _, v := range Versions(tagFormat, artifact, tags) {
		if !found || latest.Less(v) {
			latest, found = v, true
		}
	}
	return latest, found
}
