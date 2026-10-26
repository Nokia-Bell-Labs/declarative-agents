// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package hierarchy

import (
	"sort"
	"strings"
)

// Progress is a planning item's story points done over the points of all
// the agreed stories beneath it (srd045 R3.3).
type Progress struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// RollUp computes the progress of every theme, initiative, epic, and feature
// among docs from the stories done, found through Parent links; a theme
// sums its initiatives. A story whose parent is a pinned URL into another
// repository cannot be attached here, so it is listed in external and
// counted nowhere (srd045 R3.4).
func RollUp(docs []*Doc, done map[string]bool) (progress []Progress, external []string) {
	byID := map[string]*Doc{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	sums := map[string]*Progress{}
	for _, d := range docs {
		if d.Kind != Story {
			sums[d.ID] = &Progress{ID: d.ID, Kind: d.Kind}
		}
	}
	for _, s := range docs {
		if s.Kind != Story || s.Status != "agreed" {
			continue
		}
		if strings.Contains(s.Parent, "://") {
			external = append(external, s.ID)
			continue
		}
		points := s.PointsValue()
		seen := map[string]bool{}
		for id := s.Parent; id != "" && !seen[id]; {
			seen[id] = true
			p, ok := byID[id]
			if !ok || p.Kind == Theme {
				break
			}
			sums[id].Total += points
			if done[s.ID] {
				sums[id].Done += points
			}
			id = p.Parent
		}
	}
	for _, d := range docs {
		if d.Kind != Initiative || d.Parent == "" {
			continue
		}
		if t, ok := sums[d.Parent]; ok && byID[d.Parent].Kind == Theme {
			t.Done += sums[d.ID].Done
			t.Total += sums[d.ID].Total
		}
	}
	for _, p := range sums {
		progress = append(progress, *p)
	}
	sort.Slice(progress, func(i, j int) bool { return progress[i].ID < progress[j].ID })
	sort.Strings(external)
	return progress, external
}
