// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package timeline

import (
	"fmt"
	"strings"
	"time"
)

// weekMonday is the Monday of ISO week w of year: week 1 is the week
// holding January 4.
func weekMonday(year, week int) time.Time {
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	return jan4.AddDate(0, 0, -((int(jan4.Weekday())+6)%7)+7*(week-1))
}

// weeksIn is how many ISO weeks a year has: 53 when December 28 falls in
// week 53, else 52.
func weeksIn(year int) int {
	_, w := time.Date(year, time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	return w
}

func parseWeek(s string, year int) (int, int, error) {
	s = strings.TrimSpace(s)
	var y, w int
	if _, err := fmt.Sscanf(s, "%d-W%d", &y, &w); err != nil {
		if year == 0 {
			return 0, 0, fmt.Errorf("%q is no ISO week like 2026-W43", s)
		}
		if _, err := fmt.Sscanf(s, "W%d", &w); err != nil {
			return 0, 0, fmt.Errorf("%q is no ISO week like W45 or 2026-W45", s)
		}
		y = year
	}
	if w < 1 || w > weeksIn(y) {
		return 0, 0, fmt.Errorf("%q: %d has no week %d", s, y, w)
	}
	return y, w, nil
}

// ParseWeekRange reads a sprint's ISO weeks as a road map or a milestone
// writes them, 2026-W43..W45, 2026-W52..2027-W02, 2026-W52..W02, or a
// single 2026-W43, into the Monday the range starts and the Friday it ends,
// the last working day a sprint has (srd001 R7.1). A second week written
// without its year is in the first's year, or the next when it is earlier.
func ParseWeekRange(s string) (start, end time.Time, err error) {
	first, last, ranged := strings.Cut(strings.TrimSpace(s), "..")
	y1, w1, err := parseWeek(first, 0)
	if err != nil {
		return start, end, err
	}
	y2, w2 := y1, w1
	if ranged {
		if y2, w2, err = parseWeek(last, y1); err != nil {
			return start, end, err
		}
		if !strings.Contains(last, "-W") && w2 < w1 {
			y2++
			if w2 > weeksIn(y2) {
				return start, end, fmt.Errorf("%q: %d has no week %d", s, y2, w2)
			}
		}
	}
	start, end = weekMonday(y1, w1), weekMonday(y2, w2).AddDate(0, 0, 4)
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("%q ends before it starts", s)
	}
	return start, end, nil
}
