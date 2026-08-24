// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package checkpoint

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/doltsql"

// RegisterDriver registers the persistent checkpoint SQL driver. cmd/agent
// calls this once at process start; the package does not register at import
// time (srd036-dolt-state-persistence R1.3, R1.4).
func RegisterDriver() {
	doltsql.RegisterDriver()
}
