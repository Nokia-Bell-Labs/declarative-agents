// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"database/sql"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type machineRow struct {
	currentState, lastSignal       string
	iteration, tokensIn, tokensOut int
	totalCost                      float64
	conversation                   *string
}

type transitionRow struct{ fromState, signal, toState string }

type execRow struct {
	iteration   int
	ts          string
	commandName string
	label       *string
}

type resultRow struct {
	signal                      string
	output, errStr              *string
	redactionVersion            *int64
	redactedPaths, status       *string
	costDuration                int64
	costTokensIn, costTokensOut int
	costDollars                 float64
}

type fakeStore struct {
	machines    map[string]machineRow
	transitions map[string]transitionRow
	steps       map[string]execRow
	results     map[string]resultRow
	receipts    map[string]*string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		machines:    map[string]machineRow{},
		transitions: map[string]transitionRow{},
		steps:       map[string]execRow{},
		results:     map[string]resultRow{},
		receipts:    map[string]*string{},
	}
}

func (s *fakeStore) clone() *fakeStore {
	c := newFakeStore()
	for k, v := range s.machines {
		c.machines[k] = v
	}
	for k, v := range s.transitions {
		c.transitions[k] = v
	}
	for k, v := range s.steps {
		c.steps[k] = v
	}
	for k, v := range s.results {
		c.results[k] = v
	}
	for k, v := range s.receipts {
		c.receipts[k] = v
	}
	return c
}

type fakeCommit struct {
	hash    string
	message string
	snap    *fakeStore
}

type fakeDB struct {
	store    *fakeStore
	branches map[string]bool
	current  string
	commits  []fakeCommit
	calls    []string
	// failOn, when set, makes Exec return an error for any query containing it,
	// so a test can force a fault between the two per-step table writes.
	failOn string
	// outputBytes accumulates the size of every output blob written to
	// tool_outputs, so a benchmark can measure the per-step write-layer cost.
	outputBytes            int
	executionStepsExists   bool
	executionStepsHasLabel bool
	toolOutputsExists      bool
	redactionColumns       map[string]bool
	toolOutputArgs         [][]any
}

func newFakeDB() *fakeDB {
	return &fakeDB{
		store:            newFakeStore(),
		branches:         map[string]bool{"main": true},
		current:          "main",
		redactionColumns: map[string]bool{},
	}
}

func rowKey(runID string, step int) string { return runID + "|" + strconv.Itoa(step) }

func (f *fakeDB) Begin() (Transaction, error) { return &fakeTx{db: f}, nil }

func (f *fakeDB) Close() error { return nil }

func (f *fakeDB) Exec(query string, args ...any) error {
	f.calls = append(f.calls, query)
	if f.failOn != "" && strings.Contains(query, f.failOn) {
		return sql.ErrConnDone
	}
	switch {
	case strings.Contains(query, "CREATE TABLE IF NOT EXISTS execution_steps"):
		if !f.executionStepsExists {
			f.executionStepsExists = true
			f.executionStepsHasLabel = strings.Contains(query, "label VARCHAR(255)")
		}
		return nil
	case strings.Contains(query, "CREATE TABLE IF NOT EXISTS tool_outputs"):
		if !f.toolOutputsExists {
			f.toolOutputsExists = true
			for _, column := range []string{"redaction_version", "redacted_paths", "redaction_status"} {
				f.redactionColumns[column] = strings.Contains(query, column)
			}
		}
		return nil
	case strings.Contains(query, "CREATE TABLE"):
		return nil
	case strings.Contains(query, "ALTER TABLE execution_steps ADD COLUMN label"):
		f.executionStepsHasLabel = true
		return nil
	case strings.Contains(query, "ALTER TABLE tool_outputs ADD COLUMN"):
		for _, column := range []string{"redaction_version", "redacted_paths", "redaction_status"} {
			if strings.Contains(query, "ADD COLUMN "+column+" ") {
				f.redactionColumns[column] = true
			}
		}
		return nil
	case strings.Contains(query, "DOLT_CHECKOUT('main')"):
		f.current = "main"
		return nil
	case strings.Contains(query, "DOLT_CHECKOUT('-b'"):
		b := args[0].(string)
		f.branches[b] = true
		f.current = b
		return nil
	case strings.Contains(query, "DOLT_CHECKOUT("):
		b := args[0].(string)
		if !f.branches[b] {
			return sql.ErrConnDone
		}
		f.current = b
		return nil
	case strings.Contains(query, "DOLT_COMMIT"):
		msg := args[len(args)-1].(string)
		f.commits = append(f.commits, fakeCommit{
			hash:    "hash-" + strconv.Itoa(len(f.commits)+1),
			message: msg,
			snap:    f.store.clone(),
		})
		return nil
	case strings.Contains(query, "DOLT_MERGE"):
		return nil
	case strings.Contains(query, "DOLT_BRANCH('-d'"):
		delete(f.branches, args[0].(string))
		return nil
	case strings.Contains(query, "DOLT_RESET"):
		hash := args[0].(string)
		for _, c := range f.commits {
			if c.hash == hash {
				f.store = c.snap.clone()
				return nil
			}
		}
		return sql.ErrNoRows
	case strings.Contains(query, "DELETE FROM receipts"):
		deleteRowsAtOrAfter(f.store.receipts, args[0].(string), args[1].(int))
		return nil
	case strings.Contains(query, "DELETE FROM tool_outputs"):
		deleteRowsAtOrAfter(f.store.results, args[0].(string), args[1].(int))
		return nil
	case strings.Contains(query, "DELETE FROM execution_steps"):
		deleteRowsAtOrAfter(f.store.steps, args[0].(string), args[1].(int))
		return nil
	case strings.Contains(query, "DELETE FROM transitions"):
		deleteRowsAtOrAfter(f.store.transitions, args[0].(string), args[1].(int))
		return nil
	case strings.Contains(query, "REPLACE INTO machines"):
		f.store.machines[args[0].(string)] = machineRow{
			currentState: args[1].(string),
			lastSignal:   args[2].(string),
			iteration:    args[3].(int),
			tokensIn:     args[4].(int),
			tokensOut:    args[5].(int),
			totalCost:    args[6].(float64),
			conversation: strPtr(args[7]),
		}
		return nil
	case strings.Contains(query, "REPLACE INTO transitions"):
		f.store.transitions[rowKey(args[0].(string), args[1].(int))] = transitionRow{
			fromState: args[2].(string), signal: args[3].(string), toState: args[4].(string),
		}
		return nil
	case strings.Contains(query, "REPLACE INTO execution_steps"):
		f.store.steps[rowKey(args[0].(string), args[1].(int))] = execRow{
			iteration: args[2].(int), ts: args[3].(string), commandName: args[4].(string), label: strPtr(args[5]),
		}
		return nil
	case strings.Contains(query, "REPLACE INTO tool_outputs"):
		f.toolOutputArgs = append(f.toolOutputArgs, append([]any(nil), args...))
		if s, ok := args[3].(string); ok {
			f.outputBytes += len(s)
		}
		version := int64(args[5].(int))
		f.store.results[rowKey(args[0].(string), args[1].(int))] = resultRow{
			signal:           args[2].(string),
			output:           strPtr(args[3]),
			errStr:           strPtr(args[4]),
			redactionVersion: &version,
			redactedPaths:    strPtr(args[6]),
			status:           strPtr(args[7]),
			costDuration:     args[8].(int64),
			costTokensIn:     args[9].(int),
			costTokensOut:    args[10].(int),
			costDollars:      args[11].(float64),
		}
		return nil
	case strings.Contains(query, "REPLACE INTO receipts"):
		f.store.receipts[rowKey(args[0].(string), args[1].(int))] = strPtr(args[2])
		return nil
	}
	return nil
}

func deleteRowsAtOrAfter[T any](rows map[string]T, runID string, step int) {
	prefix := runID + "|"
	for key := range rows {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		index, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
		if err == nil && index >= step {
			delete(rows, key)
		}
	}
}

func (f *fakeDB) QueryRow(query string, args ...any) Scanner {
	f.calls = append(f.calls, query)
	switch {
	case strings.Contains(query, "information_schema.columns"):
		count := 0
		if strings.Contains(query, "table_name = 'execution_steps'") && f.executionStepsHasLabel {
			count = 1
		}
		for column, exists := range f.redactionColumns {
			if strings.Contains(query, "table_name = 'tool_outputs'") &&
				strings.Contains(query, "column_name = '"+column+"'") && exists {
				count = 1
			}
		}
		return &fakeScanner{kind: "count", count: count}
	case strings.Contains(query, "FROM machines"):
		m, ok := f.store.machines[args[0].(string)]
		return &fakeScanner{kind: "machine", machine: m, missing: !ok}
	case strings.Contains(query, "FROM dolt_log"):
		prefix := strings.TrimSuffix(args[0].(string), "%")
		for i := len(f.commits) - 1; i >= 0; i-- {
			if strings.HasPrefix(f.commits[i].message, prefix) {
				return &fakeScanner{kind: "log", hash: f.commits[i].hash}
			}
		}
		return &fakeScanner{kind: "log", missing: true}
	}
	return &fakeScanner{missing: true}
}

func (f *fakeDB) Query(query string, args ...any) (Rows, error) {
	f.calls = append(f.calls, query)
	runID := args[0].(string)
	var joined []joinRow
	for k, es := range f.store.steps {
		if !strings.HasPrefix(k, runID+"|") {
			continue
		}
		step, _ := strconv.Atoi(strings.TrimPrefix(k, runID+"|"))
		tr := f.store.transitions[k]
		r := f.store.results[k]
		joined = append(joined, joinRow{
			stepIndex: step, iteration: es.iteration, ts: es.ts, commandName: es.commandName, label: es.label,
			fromState: tr.fromState, toState: tr.toState, signal: tr.signal,
			resSignal: r.signal, output: r.output, errStr: r.errStr,
			redactionVersion: r.redactionVersion, redactedPaths: r.redactedPaths, redactionStatus: r.status,
			receipt:      f.store.receipts[k],
			costDuration: r.costDuration, costTokensIn: r.costTokensIn, costTokensOut: r.costTokensOut, costDollars: r.costDollars,
		})
	}
	sort.Slice(joined, func(i, j int) bool { return joined[i].stepIndex < joined[j].stepIndex })
	return &fakeRows{rows: joined, idx: -1}, nil
}

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
}

func (s *fakeScanner) Scan(dest ...any) error {
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
