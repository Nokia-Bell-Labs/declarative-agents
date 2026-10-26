// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

// An agreed epic keeps its components in transition while its points are
// not all done, or while its horizon runs past the work the latest release
// delivered: a long epic whose first sprint's stories are done still has
// stories to write (GH-114).
func TestOpenWorkReadsPointsAndHorizon(t *testing.T) {
	released := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	long := &hierarchy.Doc{ID: "ep002", Kind: hierarchy.Epic, Horizon: "2026-W43..2026-W51"}
	short := &hierarchy.Doc{ID: "ep003", Kind: hierarchy.Epic, Horizon: "2026-W43..2026-W45"}
	byID := map[string]*hierarchy.Doc{long.ID: long, short.ID: short}
	for _, c := range []struct {
		doc         *hierarchy.Doc
		done, total int
		released    time.Time
		want        bool
	}{
		{long, 8, 8, released, true},
		{long, 0, 0, released, true},
		{short, 8, 8, released, false},
		{short, 5, 8, released, true},
		{short, 0, 0, released, false},
		{short, 8, 8, time.Time{}, true},
	} {
		sd := StatusDocument{ID: c.doc.ID, Done: c.done, Total: c.total}
		if got := openWork(c.doc, sd, byID, c.released); got != c.want {
			t.Errorf("%s %d of %d, released %v: open = %v, want %v", c.doc.ID, c.done, c.total, c.released.Format(time.DateOnly), got, c.want)
		}
	}
}
