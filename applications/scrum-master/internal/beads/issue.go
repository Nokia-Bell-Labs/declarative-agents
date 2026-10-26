// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package beads is scrum-master's tracker over beads (srd042). Issues live in bd's
// JSONL interchange format, one issue per line, in a dedicated tracker
// repository that every working clone synchronizes with through git by the
// discipline of eng04-beads-sync: pull before read, push after write,
// conflicts resolved by rule.
package beads

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// IssuesFile is where the tracker repository holds its issues, the path
// bd import reads by default, so the repository stays usable by bd itself.
const IssuesFile = ".beads/issues.jsonl"

// bd's issue statuses.
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusClosed     = "closed"
)

// ErrNotFound is returned for an issue the tracker does not hold.
var ErrNotFound = errors.New("not found")

// Comment is one comment on an issue, in bd's form. Comments are
// append-only, so two clones' comments never conflict.
type Comment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// Issue is one line of the tracker file. The fields up to Comments are
// bd's own; the rest are scrum-master's, which bd carries in the file but does not
// read. State is the srd041 state, held explicitly so a merge can replay a
// transition through the table (srd042 R1.2), and StateActor is the actor
// that fired the transition into it.
type Issue struct {
	Type        string   `json:"_type,omitempty"`
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	IssueType   string   `json:"issue_type,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	CreatedBy   string   `json:"created_by,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	ClosedAt    string   `json:"closed_at,omitempty"`
	CloseReason string   `json:"close_reason,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	// Dependencies are the issue's edges in bd's form, from its link lines
	// (srd007 R6.1).
	Dependencies []Dependency `json:"dependencies,omitempty"`
	Comments     []Comment    `json:"comments,omitempty"`

	State      string `json:"state,omitempty"`
	StateActor string `json:"state_actor,omitempty"`
	// Repository is the code repository the issue belongs to, the field a
	// project path maps to (srd042 R2.4), and IID its number there,
	// unique within the repository.
	Repository string `json:"repository,omitempty"`
	IID        int    `json:"iid,omitempty"`
	// Release is the release or sprint identifier a milestone maps to
	// (srd042 R2.2).
	Release string `json:"release,omitempty"`
	DueDate string `json:"due_date,omitempty"`
}

// HasLabel reports whether the issue carries the label.
func (i Issue) HasLabel(label string) bool { return slices.Contains(i.Labels, label) }

func parseIssues(b []byte) ([]Issue, error) {
	var issues []Issue
	for n, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var i Issue
		if err := json.Unmarshal(line, &i); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", IssuesFile, n+1, err)
		}
		issues = append(issues, i)
	}
	return issues, nil
}

// writeIssues writes one issue per line sorted by id, so a change to one
// issue is a change to one line and git's diff and merge base stay exact.
func writeIssues(path string, issues []Issue) error {
	b, err := encodeJSONL(issues)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// encodeJSONL is bd's JSONL of the issues: one line each, sorted by ID.
func encodeJSONL(issues []Issue) ([]byte, error) {
	sorted := slices.Clone(issues)
	slices.SortFunc(sorted, func(a, b Issue) int { return strings.Compare(a.ID, b.ID) })
	var buf bytes.Buffer
	for _, i := range sorted {
		if i.Type == "" {
			i.Type = "issue"
		}
		line, err := json.Marshal(i)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}
