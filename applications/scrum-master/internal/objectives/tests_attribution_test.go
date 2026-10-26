// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectives

import (
	"slices"
	"testing"
)

// A test entry may name the items it proves; TestsFor keeps an attributed
// test only for its items and an unattributed one for every item (GH-20).
func TestTestsForKeepsOnlyTheItemsTests(t *testing.T) {
	names, items := parseTests([]string{"refuses_over_budget (R2.2)", "routes_by_tier (R2.1, R2.3)", "smoke"})
	if !slices.Equal(names, []string{"refuses_over_budget", "routes_by_tier", "smoke"}) {
		t.Fatalf("names = %v", names)
	}
	c := Criterion{Tests: names, TestItems: items}
	if got := c.TestsFor([]string{"R2.1"}); !slices.Equal(got, []string{"routes_by_tier", "smoke"}) {
		t.Fatalf("TestsFor(R2.1) = %v", got)
	}
	if got := c.TestsFor([]string{"R2.2"}); !slices.Equal(got, []string{"refuses_over_budget", "smoke"}) {
		t.Fatalf("TestsFor(R2.2) = %v", got)
	}
}
