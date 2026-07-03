// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"database/sql"

	"github.com/go-sql-driver/mysql"
)

// Register the "dolt" database/sql driver at the composition root so
// core.OpenDoltCheckpoint's sql.Open("dolt", dsn) resolves. Dolt speaks the
// MySQL wire protocol, so the pure-Go MySQL driver connects to a running
// `dolt sql-server` given a MySQL DSN. The engine reaches the database only
// through the database/sql seam and never imports Dolt or MySQL types
// (srd036-dolt-state-persistence R1.3, R1.4).
func init() {
	sql.Register("dolt", &mysql.MySQLDriver{})
}
