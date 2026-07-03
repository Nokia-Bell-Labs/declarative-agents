// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- fake Database seam ---------------------------------------------------
//
// fakeDB is a tiny versioned store standing in for Dolt behind the Database
// seam. It stores rows in a flat working set, records every CALL so tests can
// assert branch-per-run / commit-per-step / merge behaviour, and snapshots the
// working set on each DOLT_COMMIT so DOLT_RESET restores the target step.

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
}

type resultRow struct {
	signal                      string
	output, errStr, receipt     *string
	costDuration                int64
	costTokensIn, costTokensOut int
	costDollars                 float64
}

type fakeStore struct {
	machines    map[string]machineRow
	transitions map[string]transitionRow
	steps       map[string]execRow
	results     map[string]resultRow
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		machines:    map[string]machineRow{},
		transitions: map[string]transitionRow{},
		steps:       map[string]execRow{},
		results:     map[string]resultRow{},
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
}

func newFakeDB() *fakeDB {
	return &fakeDB{
		store:    newFakeStore(),
		branches: map[string]bool{"main": true},
		current:  "main",
	}
}

func rowKey(runID string, step int) string { return runID + "|" + strconv.Itoa(step) }

func (f *fakeDB) Begin() (Transaction, error) { return &fakeTx{db: f}, nil }

func (f *fakeDB) Close() error { return nil }

func (f *fakeDB) Exec(query string, args ...any) error {
	f.calls = append(f.calls, query)
	switch {
	case strings.Contains(query, "CREATE TABLE"):
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
			iteration: args[2].(int), ts: args[3].(string), commandName: args[4].(string),
		}
		return nil
	case strings.Contains(query, "REPLACE INTO tool_results"):
		f.store.results[rowKey(args[0].(string), args[1].(int))] = resultRow{
			signal:        args[2].(string),
			output:        strPtr(args[3]),
			errStr:        strPtr(args[4]),
			costDuration:  args[5].(int64),
			costTokensIn:  args[6].(int),
			costTokensOut: args[7].(int),
			costDollars:   args[8].(float64),
			receipt:       strPtr(args[9]),
		}
		return nil
	}
	return nil
}

func (f *fakeDB) QueryRow(query string, args ...any) Scanner {
	switch {
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
			stepIndex: step, iteration: es.iteration, ts: es.ts, commandName: es.commandName,
			fromState: tr.fromState, toState: tr.toState, signal: tr.signal,
			resSignal: r.signal, output: r.output, errStr: r.errStr, receipt: r.receipt,
			costDuration: r.costDuration, costTokensIn: r.costTokensIn, costTokensOut: r.costTokensOut, costDollars: r.costDollars,
		})
	}
	sort.Slice(joined, func(i, j int) bool { return joined[i].stepIndex < joined[j].stepIndex })
	return &fakeRows{rows: joined, idx: -1}, nil
}

type fakeTx struct{ db *fakeDB }

func (t *fakeTx) Exec(q string, a ...any) error          { return t.db.Exec(q, a...) }
func (t *fakeTx) QueryRow(q string, a ...any) Scanner    { return t.db.QueryRow(q, a...) }
func (t *fakeTx) Query(q string, a ...any) (Rows, error) { return t.db.Query(q, a...) }
func (t *fakeTx) Commit() error                          { return nil }
func (t *fakeTx) Rollback() error                        { return nil }

type fakeScanner struct {
	kind    string
	machine machineRow
	hash    string
	missing bool
}

func (s *fakeScanner) Scan(dest ...any) error {
	if s.missing {
		return sql.ErrNoRows
	}
	switch s.kind {
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
	output, errStr, receipt               *string
	costDuration                          int64
	costTokensIn, costTokensOut           int
	costDollars                           float64
}

type fakeRows struct {
	rows []joinRow
	idx  int
}

func (r *fakeRows) Next() bool   { r.idx++; return r.idx < len(r.rows) }
func (r *fakeRows) Err() error   { return nil }
func (r *fakeRows) Close() error { return nil }

func (r *fakeRows) Scan(dest ...any) error {
	row := r.rows[r.idx]
	*dest[0].(*int) = row.stepIndex
	*dest[1].(*int) = row.iteration
	*dest[2].(*string) = row.ts
	*dest[3].(*string) = row.commandName
	*dest[4].(*string) = row.fromState
	*dest[5].(*string) = row.toState
	*dest[6].(*string) = row.signal
	*dest[7].(*string) = row.resSignal
	*dest[8].(*sql.NullString) = nsFromPtr(row.output)
	*dest[9].(*sql.NullString) = nsFromPtr(row.errStr)
	*dest[10].(*int64) = row.costDuration
	*dest[11].(*int) = row.costTokensIn
	*dest[12].(*int) = row.costTokensOut
	*dest[13].(*float64) = row.costDollars
	*dest[14].(*sql.NullString) = nsFromPtr(row.receipt)
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

func countCalls(calls []string, substr string) int {
	n := 0
	for _, c := range calls {
		if strings.Contains(c, substr) {
			n++
		}
	}
	return n
}

// --- tests ----------------------------------------------------------------

func sampleExecution() Execution {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	return Execution{
		{
			Iteration: 1, Timestamp: ts, CommandName: "invoke",
			FromState: "Start", ToState: "Working", Signal: LLMResponded,
			Result:  ResultDigest{Signal: LLMResponded, Output: "hi", Cost: Cost{Duration: 2 * time.Second, TokensIn: 10, TokensOut: 5, Dollars: 0.01}},
			Receipt: `{"file":"a.txt"}`,
		},
		{
			Iteration: 2, Timestamp: ts.Add(time.Second), CommandName: "read",
			FromState: "Working", ToState: "Done", Signal: TaskCompleted,
			Result:  ResultDigest{Signal: TaskCompleted, Output: "done", Cost: Cost{TokensIn: 3, TokensOut: 1, Dollars: 0.002}},
			Receipt: "",
		},
	}
}

func samplePosition() Position {
	return Position{
		CurrentState: "Working",
		LastSignal:   LLMResponded,
		Snapshot: AgentSnapshot{
			State:        "Working",
			Signal:       LLMResponded,
			Iteration:    1,
			TokensIn:     10,
			TokensOut:    5,
			TotalCost:    0.01,
			Conversation: json.RawMessage(`[{"role":"user","content":"hi"}]`),
		},
	}
}

func TestDoltCheckpointImplementsPort(t *testing.T) {
	t.Parallel()
	var cp Checkpoint = NewDoltCheckpoint(newFakeDB(), "run-1", nil)
	require.NotNil(t, cp)
}

func TestDoltCheckpointSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := sampleExecution()
	pos := samplePosition()

	require.NoError(t, cp.Save(pos, exec[:1]))
	require.NoError(t, cp.Save(pos, exec))

	gotPos, gotExec, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, pos.CurrentState, gotPos.CurrentState)
	require.Equal(t, pos.LastSignal, gotPos.LastSignal)
	require.Equal(t, pos.Snapshot, gotPos.Snapshot)
	require.Equal(t, exec, gotExec)
	// Receipt round-trips verbatim; the empty receipt restores empty from NULL.
	require.Equal(t, `{"file":"a.txt"}`, gotExec[0].Receipt)
	require.Equal(t, "", gotExec[1].Receipt)
}

func TestDoltCheckpointCommitPerStepAndBranchPerRun(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := sampleExecution()

	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec))

	require.Equal(t, 1, countCalls(db.calls, "DOLT_CHECKOUT('-b'"), "branch created exactly once per run")
	require.Equal(t, 2, len(db.commits), "one commit per step")
	require.Contains(t, db.commits[0].message, "step 0 signal")
	require.Contains(t, db.commits[1].message, "step 1 signal")
}

func TestDoltCheckpointMergeOnTerminalState(t *testing.T) {
	t.Parallel()
	terminal := func(s State) bool { return s == "Done" }

	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", terminal)
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))
	require.Equal(t, 0, countCalls(db.calls, "DOLT_MERGE"), "non-terminal save does not merge")

	terminalPos := samplePosition()
	terminalPos.CurrentState = "Done"
	require.NoError(t, cp.Save(terminalPos, sampleExecution()))
	require.Equal(t, 1, countCalls(db.calls, "DOLT_MERGE"), "terminal save merges to main")
	require.Equal(t, 1, countCalls(db.calls, "DOLT_BRANCH('-d'"), "run branch deleted after merge")
	require.False(t, db.branches["run-1"], "run branch removed")
}

func TestDoltCheckpointRevertResetsToStepCommit(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := sampleExecution()
	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec))

	require.NoError(t, cp.Revert("run-1", 0))
	require.Equal(t, 1, countCalls(db.calls, "DOLT_RESET"), "revert resets the branch")

	// After reverting to step 0's commit, Load returns only step 0.
	_, gotExec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, gotExec, 1)
	require.Equal(t, exec[0], gotExec[0])
}

func TestDoltCheckpointRevertUnresolvedStep(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))

	err := cp.Revert("run-1", 99)
	require.ErrorIs(t, err, ErrRevertUnresolved)
}

func TestDoltCheckpointLoadNotFound(t *testing.T) {
	t.Parallel()
	cp := NewDoltCheckpoint(newFakeDB(), "missing", nil)
	_, _, err := cp.Load()
	require.ErrorIs(t, err, ErrNoCheckpoint)
}
