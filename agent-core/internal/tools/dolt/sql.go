// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

const (
	databaseExistsSQL  = "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?"
	doltStatusSQL      = "SELECT COUNT(*) FROM dolt_status"
	doltStageSQL       = "CALL DOLT_ADD('.')"
	doltCommitSQL      = "CALL DOLT_COMMIT('-m', ?)"
	doltEmptyCommitSQL = "CALL DOLT_COMMIT('--allow-empty', '-m', ?)"
)

type DatabaseOpener interface {
	Open(string) (Database, error)
}

type SQLDatabaseOpener struct{}

func (SQLDatabaseOpener) Open(dsn string) (Database, error) {
	db, err := sql.Open("dolt", dsn)
	if err != nil {
		return nil, err
	}
	return sqlDatabase{db: db}, nil
}

type Database interface {
	PingContext(context.Context) error
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (Rows, error)
	QueryRowContext(context.Context, string, ...any) Scanner
	BeginTx(context.Context) (Transaction, error)
	Close() error
}
type Transaction interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) Scanner
	Commit() error
	Rollback() error
}
type Scanner interface {
	Scan(...any) error
}
type Column struct {
	Name         string `json:"name"`
	DatabaseType string `json:"database_type,omitempty"`
	Nullable     *bool  `json:"nullable,omitempty"`
}
type Rows interface {
	Columns() ([]Column, error)
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}
type sqlDatabase struct{ db *sql.DB }

func (d sqlDatabase) PingContext(ctx context.Context) error { return d.db.PingContext(ctx) }
func (d sqlDatabase) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}
func (d sqlDatabase) QueryRowContext(ctx context.Context, query string, args ...any) Scanner {
	return d.db.QueryRowContext(ctx, query, args...)
}
func (d sqlDatabase) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqlRows{rows: rows}, nil
}
func (d sqlDatabase) BeginTx(ctx context.Context) (Transaction, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlTransaction{tx: tx}, nil
}
func (d sqlDatabase) Close() error { return d.db.Close() }

type sqlTransaction struct{ tx *sql.Tx }

func (t sqlTransaction) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}
func (t sqlTransaction) QueryRowContext(ctx context.Context, query string, args ...any) Scanner {
	return t.tx.QueryRowContext(ctx, query, args...)
}
func (t sqlTransaction) Commit() error   { return t.tx.Commit() }
func (t sqlTransaction) Rollback() error { return t.tx.Rollback() }

type sqlRows struct{ rows *sql.Rows }

func (r sqlRows) Columns() ([]Column, error) {
	types, err := r.rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	columns := make([]Column, len(types))
	for i, typ := range types {
		columns[i] = Column{Name: typ.Name(), DatabaseType: typ.DatabaseTypeName()}
		if nullable, ok := typ.Nullable(); ok {
			columns[i].Nullable = &nullable
		}
	}
	return columns, nil
}
func (r sqlRows) Next() bool             { return r.rows.Next() }
func (r sqlRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r sqlRows) Err() error             { return r.rows.Err() }
func (r sqlRows) Close() error           { return r.rows.Close() }
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
