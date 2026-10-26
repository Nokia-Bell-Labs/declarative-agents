// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package legacyenv

import (
	"os"
	"slices"
	"testing"
)

// A TPM_ variable fills its SCRUM_MASTER_ name when that is unset, and
// never overrides one that is set (GH-38).
func TestAdoptReadsTheOldNamesUnderTheNew(t *testing.T) {
	t.Setenv("TPM_LEGACYENV_ONLY", "old")
	t.Setenv("TPM_LEGACYENV_BOTH", "old")
	t.Setenv("SCRUM_MASTER_LEGACYENV_BOTH", "new")
	t.Setenv("SCRUM_MASTER_LEGACYENV_ONLY", "")
	os.Unsetenv("SCRUM_MASTER_LEGACYENV_ONLY")

	Adopt()
	if got := os.Getenv("SCRUM_MASTER_LEGACYENV_ONLY"); got != "old" {
		t.Errorf("SCRUM_MASTER_LEGACYENV_ONLY = %q, want the TPM_ value", got)
	}
	if got := os.Getenv("SCRUM_MASTER_LEGACYENV_BOTH"); got != "new" {
		t.Errorf("SCRUM_MASTER_LEGACYENV_BOTH = %q, want the new name's own value", got)
	}
	if set := Set(); !slices.Contains(set, "TPM_LEGACYENV_ONLY") || !slices.Contains(set, "TPM_LEGACYENV_BOTH") {
		t.Errorf("Set() = %v, want both TPM_ names", set)
	}
	if got := Renamed("TPM_TRACKER"); got != "SCRUM_MASTER_TRACKER" {
		t.Errorf("Renamed(TPM_TRACKER) = %q", got)
	}
}
