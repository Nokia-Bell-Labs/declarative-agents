// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltcheckpoint

import (
	"database/sql"
	"sync"

	"github.com/go-sql-driver/mysql"
)

var registerDriverOnce sync.Once

// RegisterDriver registers the "dolt" database/sql driver so
// OpenDoltCheckpoint's sql.Open("dolt", dsn) resolves. Dolt speaks the
// MySQL wire protocol; the pure-Go MySQL driver connects to a running
// `dolt sql-server`. OpenDoltCheckpoint calls this once; the package does
// not register at import time (srd036-dolt-state-persistence R1.3, R1.4).
func RegisterDriver() {
	registerDriverOnce.Do(func() {
		sql.Register("dolt", &mysql.MySQLDriver{})
	})
}
