// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// checkoutTop says dir is the top of a checkout, so the listing is of that
// repository and pulling and sweeping its tracker are about it; a directory
// inside one, or outside any, only has its documents read.
func checkoutTop(dir string) bool {
	root, err := toplevelOf(dir)
	if err != nil {
		return false
	}
	here, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	if here, err = filepath.Abs(here); err != nil {
		return false
	}
	top, err := filepath.EvalSymlinks(root)
	return err == nil && here == top
}

// pullFirst brings the checkout at dir up to date before the listing reads it
// (srd045 R8.1), and says why not when it stays as it is: a listing of what a
// person has locally is still worth printing.
func pullFirst(dir string, dryRun bool) (note string) {
	if !checkoutTop(dir) || checkout.Origin(dir) == "" {
		return ""
	}
	if _, why := landable(dir, dryRun); why != "" {
		return "not pulled, so this is the checkout as it stands: " + why
	}
	return ""
}

// addIssues gives each story and epic its issues, read from the tracker as
// backlog record reads them; what the tracker cannot say is what the document
// records, and the notes say which that is. A tracker it cannot reach fails
// nothing, since the documents are still worth listing.
func addIssues(open func() (tracker.Tracker, error), dir string, entries []Entry) (notes []string) {
	held := map[string][]LedgerEntry{}
	for _, e := range entries {
		if e.Kind != string(hierarchy.Story) && e.Kind != string(hierarchy.Epic) {
			continue
		}
		doc, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(e.Path)))
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: %v", e.ID, err))
			continue
		}
		recorded, err := LedgerOf(doc)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: its issues field does not parse: %v", e.ID, err))
			continue
		}
		held[e.ID] = recorded
	}
	read, swept, more := sweep(open, dir, len(held) > 0)
	notes = append(notes, more...)
	for i := range entries {
		recorded, ok := held[entries[i].ID]
		if !ok {
			continue
		}
		entries[i].Issues = withUnswept(read[entries[i].ID], recorded, swept)
	}
	return notes
}

// sweep is the tracker's issues per document for the repository at dir, when
// dir is the top of its checkout and the repository has documents to show them
// beside.
func sweep(open func() (tracker.Tracker, error), dir string, wanted bool) (map[string][]LedgerEntry, map[string]bool, []string) {
	if !wanted || !checkoutTop(dir) {
		return nil, nil, nil
	}
	_, own, projects, notes, err := ledgerScope(dir)
	if err != nil {
		return nil, nil, []string{"the issues shown are what the documents record: " + err.Error()}
	}
	t, err := open()
	if err != nil {
		return nil, nil, append(notes, fmt.Sprintf("the tracker can't be reached, so the issues shown are what the documents record: %v", err))
	}
	read, swept, more := TrackerLedgers(t, projects, own)
	return read, swept, append(notes, more...)
}

// issueLines prints a document's issues under it, open ones first, each by the
// short reference a person reads: its repository's last segment and number.
func issueLines(issues []LedgerEntry) string {
	var b strings.Builder
	for _, state := range []string{"open", "closed"} {
		for _, e := range issues {
			if (e.State == "closed") != (state == "closed") {
				continue
			}
			fmt.Fprintf(&b, "  %-6s %s %s", state, path.Base(e.Ref), e.Title)
			if len(e.Who) > 0 {
				fmt.Fprintf(&b, " (%s)", strings.Join(e.Who, ", "))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}
