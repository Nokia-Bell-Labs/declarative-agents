// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltsql

// Shared Dolt SQL statements. Production CALL DOLT_COMMIT literals live only
// in this file so the checkpoint backend and the Dolt word cannot drift.
const (
	CommitSQL              = "CALL DOLT_COMMIT('-m', ?)"
	EmptyCommitSQL         = "CALL DOLT_COMMIT('--allow-empty', '-m', ?)"
	StageAllEmptyCommitSQL = "CALL DOLT_COMMIT('-A', '--allow-empty', '-m', ?)"
	StageSQL               = "CALL DOLT_ADD('.')"
	StatusSQL              = "SELECT COUNT(*) FROM dolt_status"
	DatabaseExistsSQL      = "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?"
	CheckoutSQL            = "CALL DOLT_CHECKOUT(?)"
	CheckoutMainSQL        = "CALL DOLT_CHECKOUT('main')"
	CheckoutNewBranchSQL   = "CALL DOLT_CHECKOUT('-b', ?)"
	MergeSQL               = "CALL DOLT_MERGE(?)"
	DeleteBranchSQL        = "CALL DOLT_BRANCH('-d', ?)"
	ResetHardSQL           = "CALL DOLT_RESET('--hard', ?)"
	HeadHashSQL            = "SELECT HASHOF('HEAD')"
)
