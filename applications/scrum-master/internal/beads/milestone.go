// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"path/filepath"
	"slices"
	"strings"
)

// MilestonesFile is where the tracker repository holds milestones, beside
// the issues; bd does not read it.
const MilestonesFile = ".beads/milestones.jsonl"

// Milestone is a group's or project's milestone (srd042 R2.2): Scope is the
// group or project path it belongs to, and an issue names it by title in its
// Release field.
type Milestone struct {
	Scope       string `json:"scope"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func (m Milestone) key() string     { return m.Scope + "\x00" + m.Title }
func (m Milestone) changed() string { return m.UpdatedAt }

// ancestors returns project and every group above it, nearest first.
func ancestors(project string) []string {
	var out []string
	for p := project; p != ""; {
		out = append(out, p)
		i := strings.LastIndex(p, "/")
		if i < 0 {
			break
		}
		p = p[:i]
	}
	return out
}

// Milestones pulls the tracker repository and returns every milestone.
func (s *Store) Milestones() ([]Milestone, Freshness, error) {
	fr, err := s.Sync()
	if err != nil {
		return nil, fr, err
	}
	ms, err := readRecords[Milestone](s, MilestonesFile)
	return ms, fr, err
}

// EnsureMilestone makes the milestone titled m.Title in m.Scope carry m's
// description and dates, creating it when the scope has none, and reports
// created, updated, or unchanged (srd042 R2.2).
func (s *Store) EnsureMilestone(m Milestone) (string, error) {
	result := "unchanged"
	_, _, _, err := s.writeFiles("milestone "+m.Scope+" "+m.Title, func() (bool, error) {
		ms, err := readRecords[Milestone](s, MilestonesFile)
		if err != nil {
			return false, err
		}
		m.UpdatedAt = s.stamp()
		n := slices.IndexFunc(ms, func(x Milestone) bool { return x.key() == m.key() })
		switch {
		case n < 0:
			ms, result = append(ms, m), "created"
		case sameRecord(ms[n], m):
			return false, nil
		default:
			ms[n], result = m, "updated"
		}
		return true, writeRecords(filepath.Join(s.Dir, MilestonesFile), ms)
	})
	return result, err
}
