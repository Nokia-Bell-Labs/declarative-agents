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
//
// The single-importer-unit class enforces the fragment rule (GH-2079): a unit
// needs two or more importers or an instantiation, otherwise it is an include
// by another name and folds back into its one importer (GH-2107). Two kinds of
// unit are outside it. A type-only unit cannot fold back, because a declaration
// may not carry both tools and types. agent-core's library under tools/ is
// published for importers outside this repository, so an in-repo count does
// not measure it. The class had no entries when it landed, so any new
// single-importer unit fails.
package declstyle
