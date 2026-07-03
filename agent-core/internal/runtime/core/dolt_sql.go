// Copyright (c) 2026 Nokia. All rights reserved.

package core

import "database/sql"

// newSQLDatabase bridges a *sql.DB to the Database seam using only the standard
// library, so the Dolt driver stays at the composition root and core never
// imports Dolt (srd036-dolt-state-persistence R1.2, R1.3).
func newSQLDatabase(db *sql.DB) Database { return &sqlDatabase{db: db} }

type sqlDatabase struct{ db *sql.DB }

func (s *sqlDatabase) Begin() (Transaction, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	return &sqlTransaction{tx: tx}, nil
}

func (s *sqlDatabase) Exec(query string, args ...any) error {
	_, err := s.db.Exec(query, args...)
	return err
}

func (s *sqlDatabase) QueryRow(query string, args ...any) Scanner {
	return s.db.QueryRow(query, args...)
}

func (s *sqlDatabase) Query(query string, args ...any) (Rows, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *sqlDatabase) Close() error { return s.db.Close() }

type sqlTransaction struct{ tx *sql.Tx }

func (t *sqlTransaction) Exec(query string, args ...any) error {
	_, err := t.tx.Exec(query, args...)
	return err
}

func (t *sqlTransaction) QueryRow(query string, args ...any) Scanner {
	return t.tx.QueryRow(query, args...)
}

func (t *sqlTransaction) Query(query string, args ...any) (Rows, error) {
	rows, err := t.tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (t *sqlTransaction) Commit() error   { return t.tx.Commit() }
func (t *sqlTransaction) Rollback() error { return t.tx.Rollback() }
