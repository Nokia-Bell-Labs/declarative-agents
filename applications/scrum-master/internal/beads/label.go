// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"fmt"
	"path/filepath"
	"slices"
)

// LabelsFile is where the tracker repository holds the label schema, beside
// the issues; bd does not read it.
const LabelsFile = ".beads/labels.jsonl"

// Label is a label defined on a group, as scrum-master issue labels manages it.
type Label struct {
	Scope       string `json:"scope"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func (l Label) key() string     { return l.Scope + "\x00" + l.Name }
func (l Label) changed() string { return l.UpdatedAt }

// Labels pulls the tracker repository and returns every label.
func (s *Store) Labels() ([]Label, Freshness, error) {
	fr, err := s.Sync()
	if err != nil {
		return nil, fr, err
	}
	ls, err := readRecords[Label](s, LabelsFile)
	return ls, fr, err
}

// PutLabel creates l, or with update replaces the label of its name; it
// refuses to create one that exists or update one that does not.
func (s *Store) PutLabel(l Label, update bool) error {
	_, _, _, err := s.writeFiles("label "+l.Scope+" "+l.Name, func() (bool, error) {
		ls, err := readRecords[Label](s, LabelsFile)
		if err != nil {
			return false, err
		}
		l.UpdatedAt = s.stamp()
		n := slices.IndexFunc(ls, func(x Label) bool { return x.key() == l.key() })
		switch {
		case n >= 0 && !update:
			return false, fmt.Errorf("label %q already exists in %s", l.Name, l.Scope)
		case n < 0 && update:
			return false, fmt.Errorf("label %q in %s: %w", l.Name, l.Scope, ErrNotFound)
		case n < 0:
			ls = append(ls, l)
		default:
			ls[n] = l
		}
		return true, writeRecords(filepath.Join(s.Dir, LabelsFile), ls)
	})
	return err
}
