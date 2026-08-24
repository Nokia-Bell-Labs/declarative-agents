// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltsql

import (
	"database/sql"
	"sync"

	"github.com/go-sql-driver/mysql"
)

var registerDriverOnce sync.Once

// RegisterDriver registers the "dolt" database/sql driver so sql.Open("dolt",
// dsn) resolves. Dolt speaks the MySQL wire protocol; the pure-Go MySQL driver
// connects to a running `dolt sql-server`. Open and OpenDB call this once; the
// package does not register at import time (srd036-dolt-state-persistence R1.3,
// R1.4).
func RegisterDriver() {
	registerDriverOnce.Do(func() {
		sql.Register("dolt", &mysql.MySQLDriver{})
	})
}
