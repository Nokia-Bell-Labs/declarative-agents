// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltsql

import (
	"context"
	"database/sql"
)

// Database is the context-flavored database/sql seam shared by the checkpoint
// backend and the Dolt boundary words.
type Database interface {
	PingContext(context.Context) error
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (Rows, error)
	QueryRowContext(context.Context, string, ...any) Scanner
	BeginTx(context.Context) (Transaction, error)
	Close() error
}

// Transaction is one atomic unit of work.
type Transaction interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) Scanner
	QueryContext(context.Context, string, ...any) (Rows, error)
	Commit() error
	Rollback() error
}

// Scanner reads a single row's columns into destinations.
type Scanner interface {
	Scan(...any) error
}

// Column describes one result-set field.
type Column struct {
	Name         string `json:"name"`
	DatabaseType string `json:"database_type,omitempty"`
	Nullable     *bool  `json:"nullable,omitempty"`
}

// Rows iterates a multi-row result.
type Rows interface {
	Columns() ([]Column, error)
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}

// OpenDB registers the Dolt driver and opens a *sql.DB for the DSN.
func OpenDB(dsn string) (*sql.DB, error) {
	RegisterDriver()
	return sql.Open("dolt", dsn)
}

// Open registers the Dolt driver and returns a Database over the DSN.
func Open(dsn string) (Database, error) {
	db, err := OpenDB(dsn)
	if err != nil {
		return nil, err
	}
	return Wrap(db), nil
}

// Wrap adapts a *sql.DB to the Database seam.
func Wrap(db *sql.DB) Database { return sqlDatabase{db: db} }

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

func (t sqlTransaction) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqlRows{rows: rows}, nil
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
