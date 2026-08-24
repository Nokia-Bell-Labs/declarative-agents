// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import (
	"fmt"

	"github.com/go-sql-driver/mysql"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/doltsql"
)

type DatabaseOpener interface {
	Open(string) (Database, error)
}

type SQLDatabaseOpener struct{}

func (SQLDatabaseOpener) Open(dsn string) (Database, error) {
	return doltsql.Open(dsn)
}

type (
	Database    = doltsql.Database
	Transaction = doltsql.Transaction
	Scanner     = doltsql.Scanner
	Column      = doltsql.Column
	Rows        = doltsql.Rows
)

const (
	databaseExistsSQL  = doltsql.DatabaseExistsSQL
	doltStatusSQL      = doltsql.StatusSQL
	doltStageSQL       = doltsql.StageSQL
	doltCommitSQL      = doltsql.CommitSQL
	doltEmptyCommitSQL = doltsql.EmptyCommitSQL
)

func operationDSNs(raw, database string) (string, string, DatabaseIdentity, error) {
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		return "", "", DatabaseIdentity{}, fmt.Errorf("invalid configured connection")
	}
	identity, err := IdentityFromDSN(raw, database)
	if err != nil {
		return "", "", DatabaseIdentity{}, err
	}
	cfg.MultiStatements = false
	cfg.DBName = ""
	server := cfg.FormatDSN()
	cfg.DBName = database
	return server, cfg.FormatDSN(), identity, nil
}
