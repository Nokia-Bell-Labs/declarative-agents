// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package declstyle makes the remaining pre-type-system declaration forms
// visible and stops new ones being added while the migration runs (GH-1974).
// It carries no production code; the gate lives in legacy_test.go, mirroring
// the gostyle size constitution.
//
// The baseline only shrinks. A legacy form not in it fails as new, and an
// entry no longer present fails as stale, so converting a file means deleting
// its lines in the same change. GH-1971 flips the type system strict once the
// baseline is empty.
package declstyle
