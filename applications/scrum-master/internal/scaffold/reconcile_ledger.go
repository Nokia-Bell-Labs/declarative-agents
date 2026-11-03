// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// ledgerFindings checks each story's and epic's issues record in the scrum
// repository's checkout against the tracking system (srd043 R8.4): an
// implementing issue with no entry, an entry whose issue the tracking
// system does not show, and an entry whose state or closed date lags are
// each a finding naming scrum-master backlog record as the remedy. An
// entry whose issue lives in a project the sweep could not read is left
// unjudged, with a note.
func ledgerFindings(t tracker.Tracker, dir string, sub program.Subsystem, scrum string) ([]string, []string) {
	want, swept, notes := backlog.TrackerLedgers(t, sub.Repositories(), scrum)
	paths, err := backlog.LedgerDocuments(dir)
	if err != nil {
		return nil, notes
	}
	var findings []string
	remedy := "; scrum-master backlog record writes it"
	for _, p := range paths {
		id := strings.TrimSuffix(filepath.Base(p), ".md")
		doc, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		held, err := backlog.LedgerOf(doc)
		if err != nil {
			findings = append(findings, fmt.Sprintf("%s's issues field does not parse: %v%s", id, err, remedy))
			continue
		}
		heldBy := map[string]backlog.LedgerEntry{}
		for _, e := range held {
			heldBy[e.Ref] = e
		}
		for _, w := range want[id] {
			h, ok := heldBy[w.Ref]
			switch {
			case !ok:
				findings = append(findings, fmt.Sprintf("%s implements %s with no entry in its issues record%s", w.Ref, id, remedy))
			case h.State != w.State || h.Closed != w.Closed:
				findings = append(findings, fmt.Sprintf("%s's issues record holds %s as %s, but the tracking system has it %s%s", id, w.Ref, state(h), state(w), remedy))
			}
			delete(heldBy, w.Ref)
		}
		for ref := range heldBy {
			project, _, _ := strings.Cut(ref, "#")
			if swept[project] {
				findings = append(findings, fmt.Sprintf("%s's issues record names %s, which the tracking system does not show%s", id, ref, remedy))
			}
		}
	}
	sort.Strings(findings)
	return findings, notes
}

// state names an entry's state with its closed day, so a lagging record
// reads as what differs.
func state(e backlog.LedgerEntry) string {
	if e.Closed != "" {
		return e.State + " on " + e.Closed
	}
	return e.State
}
