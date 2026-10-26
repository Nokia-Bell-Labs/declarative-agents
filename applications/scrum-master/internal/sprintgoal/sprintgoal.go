// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package sprintgoal writes and reads the marker tasks puts in every Sprint
// Goal issue (srd034 R3), and reads the marker of an objective issue, a
// Sprint Goal filed before release 04.0, as a Sprint Goal's (srd034 R3.7).
// A Sprint Goal is known by its marker, not by a label. It also reads a
// Sprint Goal's Sprint Backlog and keeps the record of the stories each
// sprint-end summary reports done (srd038 R2.4).
package sprintgoal

import (
	"fmt"
	"path"
	"strings"
)

const (
	prefix = "<!-- scrum-master goals "
	marker = prefix + "sprint-goal "
	legacy = prefix + "objective="
)

// Ref is what a Sprint Goal's marker records.
type Ref struct {
	Release string
	// Serves lists the initiative outcomes the Sprint Goal serves, as
	// document#group keys; an objective issue serves the one its marker names.
	Serves []string
	// Legacy is set for an objective issue filed before release 04.0.
	Legacy bool
}

// Marker identifies a team's Sprint Goal for a release, so a second push
// finds it instead of filing another (srd022 R2.3).
func Marker(release string, serves []string) string {
	return fmt.Sprintf("%srelease=%s serves=%s -->", marker, release, strings.Join(serves, ","))
}

// Is reports whether an issue description carries a Sprint Goal marker or
// an objective marker.
func Is(description string) bool {
	return strings.Contains(description, marker) || strings.Contains(description, legacy)
}

// Parse reads the marker in an issue description.
func Parse(description string) (Ref, bool) {
	if _, rest, ok := strings.Cut(description, marker); ok {
		fields := markerFields(rest)
		var serves []string
		if s := fields["serves"]; s != "" {
			serves = strings.Split(s, ",")
		}
		return Ref{Release: fields["release"], Serves: serves}, true
	}
	if _, rest, ok := strings.Cut(description, legacy); ok {
		key, rest, _ := strings.Cut(rest, " ")
		fields := markerFields(rest)
		return Ref{Release: fields["release"], Serves: []string{key}, Legacy: true}, true
	}
	return Ref{}, false
}

// markerFields reads the key=value pairs up to the end of the marker.
func markerFields(rest string) map[string]string {
	body, _, _ := strings.Cut(rest, "-->")
	out := map[string]string{}
	for _, f := range strings.Fields(body) {
		if k, v, ok := strings.Cut(f, "="); ok {
			out[k] = v
		}
	}
	return out
}

// CitedURLs maps each initiative item a Sprint Goal cites, as doc#item, to
// the pinned URL its Cites line names, so what is said of the item later
// names the same text the goal does (GH-101).
func CitedURLs(description string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(description, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Cites:")
		if !ok {
			continue
		}
		url, _, _ := strings.Cut(rest, "|")
		url = strings.TrimSpace(url)
		if u, item, ok := strings.Cut(url, "#"); ok {
			out[strings.TrimSuffix(path.Base(u), ".md")+"#"+item] = url
		}
	}
	return out
}

// CitedItems are the initiative items a Sprint Goal promised, as doc#item,
// from its Cites lines: a pinned URL's file name without .md and its
// anchor. Plan's slipped items and goals plan's carried items read the same
// list, so the two never disagree (srd031 R2.3, srd022 R2.3).
func CitedItems(description string) []string {
	var out []string
	for _, line := range strings.Split(description, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Cites:")
		if !ok {
			continue
		}
		url, _, _ := strings.Cut(rest, "|")
		if u, item, ok := strings.Cut(strings.TrimSpace(url), "#"); ok {
			out = append(out, strings.TrimSuffix(path.Base(u), ".md")+"#"+item)
		}
	}
	return out
}
