// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeTx struct{ db *fakeDB }

func (t *fakeTx) Exec(q string, a ...any) error { return t.db.Exec(q, a...) }

func (t *fakeTx) QueryRow(q string, a ...any) Scanner { return t.db.QueryRow(q, a...) }

func (t *fakeTx) Query(q string, a ...any) (Rows, error) { return t.db.Query(q, a...) }

func (t *fakeTx) Commit() error { return nil }

func (t *fakeTx) Rollback() error { return nil }

type fakeScanner struct {
	kind    string
	machine machineRow
	hash    string
	count   int
	missing bool
	scanErr error
}

func (s *fakeScanner) Scan(dest ...any) error {
	if s.scanErr != nil {
		return s.scanErr
	}
	if s.missing {
		return sql.ErrNoRows
	}
	switch s.kind {
	case "count":
		*dest[0].(*int) = s.count
	case "machine":
		*dest[0].(*string) = s.machine.currentState
		*dest[1].(*string) = s.machine.lastSignal
		*dest[2].(*int) = s.machine.iteration
		*dest[3].(*int) = s.machine.tokensIn
		*dest[4].(*int) = s.machine.tokensOut
		*dest[5].(*float64) = s.machine.totalCost
		*dest[6].(*sql.NullString) = nsFromPtr(s.machine.conversation)
		*dest[7].(*sql.NullString) = nsFromPtr(s.machine.domain)
		*dest[8].(*sql.NullString) = nsFromPtr(s.machine.iterator)
		*dest[9].(*sql.NullString) = nsFromPtr(s.machine.programProfile)
		*dest[10].(*sql.NullString) = nsFromPtr(s.machine.programDigest)
	case "log":
		*dest[0].(*string) = s.hash
	}
	return nil
}

type joinRow struct {
	stepIndex, iteration                  int
	ts, commandName                       string
	fromState, toState, signal, resSignal string
	label, output, errStr, receipt        *string
	redactionVersion                      *int64
	redactedPaths, redactionStatus        *string
	costDuration                          int64
	costTokensIn, costTokensOut           int
	costDollars                           float64
}

type fakeRows struct {
	rows []joinRow
	idx  int
}

func (r *fakeRows) Next() bool { r.idx++; return r.idx < len(r.rows) }

func (r *fakeRows) Err() error { return nil }

func (r *fakeRows) Close() error { return nil }

func (r *fakeRows) Scan(dest ...any) error {
	row := r.rows[r.idx]
	*dest[0].(*int) = row.stepIndex
	*dest[1].(*int) = row.iteration
	*dest[2].(*string) = row.ts
	*dest[3].(*string) = row.commandName
	*dest[4].(*sql.NullString) = nsFromPtr(row.label)
	*dest[5].(*string) = row.fromState
	*dest[6].(*string) = row.toState
	*dest[7].(*string) = row.signal
	*dest[8].(*string) = row.resSignal
	*dest[9].(*sql.NullString) = nsFromPtr(row.output)
	*dest[10].(*sql.NullString) = nsFromPtr(row.errStr)
	*dest[11].(*sql.NullInt64) = niFromPtr(row.redactionVersion)
	*dest[12].(*sql.NullString) = nsFromPtr(row.redactedPaths)
	*dest[13].(*sql.NullString) = nsFromPtr(row.redactionStatus)
	*dest[14].(*int64) = row.costDuration
	*dest[15].(*int) = row.costTokensIn
	*dest[16].(*int) = row.costTokensOut
	*dest[17].(*float64) = row.costDollars
	*dest[18].(*sql.NullString) = nsFromPtr(row.receipt)
	return nil
}

func strPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := v.(string)
	return &s
}

func nsFromPtr(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func niFromPtr(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func countCalls(calls []string, substr string) int {
	n := 0
	for _, c := range calls {
		if strings.Contains(c, substr) {
			n++
		}
	}
	return n
}

func lastCallIndex(calls []string, substr string) int {
	for i := len(calls) - 1; i >= 0; i-- {
		if strings.Contains(calls[i], substr) {
			return i
		}
	}
	return -1
}

func requireNoUnquotedSignalColumn(t *testing.T, query string) {
	t.Helper()
	normalized := " " + strings.Join(strings.Fields(query), " ") + " "
	for _, token := range []string{
		" signal VARCHAR",
		" from_state, signal,",
		" step_index, signal,",
		" t.signal",
		" o.signal",
		" r.signal",
	} {
		require.NotContains(t, normalized, token)
	}
}
