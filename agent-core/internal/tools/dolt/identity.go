// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/doltsql"

type DatabaseIdentity = doltsql.DatabaseIdentity

func IdentityFromDSN(dsn, database string) (DatabaseIdentity, error) {
	return doltsql.IdentityFromDSN(dsn, database)
}
