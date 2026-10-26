// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package timeline

import (
	"testing"
	"time"
)

// The ISO-week range parser reads a sprint's weeks into its Monday and
// Friday, across year boundaries and 53-week years (GH-82).
func TestParseWeekRange(t *testing.T) {
	for _, c := range []struct{ in, start, end string }{
		{"2026-W43..W45", "2026-10-19", "2026-11-06"},
		{"2026-W43..2026-W45", "2026-10-19", "2026-11-06"},
		{"2026-W43", "2026-10-19", "2026-10-23"},
		{"2026-W52..2027-W02", "2026-12-21", "2027-01-15"},
		{"2026-W52..W02", "2026-12-21", "2027-01-15"},
		{"2026-W53", "2026-12-28", "2027-01-01"}, // 2026 has 53 ISO weeks
		{"2027-W01", "2027-01-04", "2027-01-08"},
		{"2020-W53..W01", "2020-12-28", "2021-01-08"},
	} {
		s, e, err := ParseWeekRange(c.in)
		if err != nil || s.Format(time.DateOnly) != c.start || e.Format(time.DateOnly) != c.end {
			t.Errorf("%s = %s..%s, %v; want %s..%s", c.in, s.Format(time.DateOnly), e.Format(time.DateOnly), err, c.start, c.end)
		}
	}
	for _, bad := range []string{"", "01.1", "2027-W53", "2026-W45..2026-W43", "W45", "2026-W00"} {
		if _, _, err := ParseWeekRange(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
