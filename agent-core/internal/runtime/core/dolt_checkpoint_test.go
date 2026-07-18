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
	output, errStr              *string
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
	if f.failOn != "" && strings.Contains(query, f.failOn) {
		return sql.ErrConnDone
	}
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
	case strings.Contains(query, "REPLACE INTO tool_outputs"):
		f.store.results[rowKey(args[0].(string), args[1].(int))] = resultRow{
			signal:        args[2].(string),
			output:        strPtr(args[3]),
			errStr:        strPtr(args[4]),
			costDuration:  args[5].(int64),
			costTokensIn:  args[6].(int),
			costTokensOut: args[7].(int),
			costDollars:   args[8].(float64),
		}
		return nil
	case strings.Contains(query, "REPLACE INTO receipts"):
		f.store.receipts[rowKey(args[0].(string), args[1].(int))] = strPtr(args[2])
		return nil
	}
	return nil
}

func (f *fakeDB) QueryRow(query string, args ...any) Scanner {
	f.calls = append(f.calls, query)
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
			stepIndex: step, iteration: es.iteration, ts: es.ts, commandName: es.commandName,
			fromState: tr.fromState, toState: tr.toState, signal: tr.signal,
			resSignal: r.signal, output: r.output, errStr: r.errStr, receipt: f.store.receipts[k],
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

// threeStepExecution is a three-entry run where every step carries a distinct
// receipt, used to prove revert and terminal reaping across both planes.
func threeStepExecution() Execution {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	return Execution{
		{
			Iteration: 1, Timestamp: ts, CommandName: "invoke",
			FromState: "Start", ToState: "Working", Signal: LLMResponded,
			Result:  ResultDigest{Signal: LLMResponded, Output: "s0", Cost: Cost{TokensIn: 1}},
			Receipt: `{"step":0}`,
		},
		{
			Iteration: 2, Timestamp: ts.Add(time.Second), CommandName: "read",
			FromState: "Working", ToState: "Working", Signal: LLMResponded,
			Result:  ResultDigest{Signal: LLMResponded, Output: "s1", Cost: Cost{TokensIn: 2}},
			Receipt: `{"step":1}`,
		},
		{
			Iteration: 3, Timestamp: ts.Add(2 * time.Second), CommandName: "write",
			FromState: "Working", ToState: "Done", Signal: TaskCompleted,
			Result:  ResultDigest{Signal: TaskCompleted, Output: "s2", Cost: Cost{TokensIn: 3}},
			Receipt: `{"step":2}`,
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

func TestDoltCheckpointSplitsToolOutputsAndReceipts(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)

	// Two steps: step 0 carries a receipt, step 1 has an empty receipt.
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()))

	var toolOutputsCreate string
	haveReceiptsCreate := false
	for _, q := range db.calls {
		if strings.Contains(q, "CREATE TABLE IF NOT EXISTS tool_outputs") {
			toolOutputsCreate = q
		}
		if strings.Contains(q, "CREATE TABLE IF NOT EXISTS receipts") {
			haveReceiptsCreate = true
		}
	}
	require.NotEmpty(t, toolOutputsCreate, "tool_outputs table is created")
	require.NotContains(t, toolOutputsCreate, "receipt", "tool_outputs must not carry a receipt column; the split cannot silently regress")
	require.True(t, haveReceiptsCreate, "receipts table is created")

	// The forward plane gets a row per step; the reverse plane gets a row only
	// for the step that carried a receipt, so an empty receipt is row absence.
	require.Equal(t, 2, countCalls(db.calls, "REPLACE INTO tool_outputs"), "one forward-plane row per step")
	require.Equal(t, 1, countCalls(db.calls, "REPLACE INTO receipts"), "only the step with a receipt writes a reverse-plane row")
}

func TestDoltCheckpointSingleTransactionAtomicity(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	db.failOn = "REPLACE INTO receipts"
	cp := NewDoltCheckpoint(db, "run-1", nil)

	// step 0 carries a receipt, so the receipts write is reached and forced to
	// fail between the two per-step table writes.
	err := cp.Save(samplePosition(), sampleExecution()[:1])
	require.Error(t, err, "a fault on the receipts write fails the save")
	require.Equal(t, 0, countCalls(db.calls, "DOLT_COMMIT"), "no commit is issued when a step is only partially written")
}

func TestDoltCheckpointQuotesReservedSignalColumn(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)

	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))
	_, _, err := cp.Load()
	require.NoError(t, err)

	queries := strings.Join(db.calls, "\n")
	require.Equal(t, 2, strings.Count(queries, "`signal` VARCHAR(255) NOT NULL"))
	require.Contains(t, queries, "REPLACE INTO transitions (run_id, step_index, from_state, `signal`, to_state)")
	require.Contains(t, queries, "(run_id, step_index, `signal`, output, error")
	require.Contains(t, queries, "t.`signal`")
	require.Contains(t, queries, "o.`signal`")
	requireNoUnquotedSignalColumn(t, queries)
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

// TestDoltCheckpointRevertReapsBothPlanes proves invariant (1): reverting to an
// earlier step reaps both the tool_outputs forward plane and the receipts reverse
// plane for every later step, by branch construction and without a separate
// invalidation walk (srd036-dolt-state-persistence R4, srd038-command-state-store
// R4). Implements the reap half of rel07.0-uc002-two-plane-revert.
func TestDoltCheckpointRevertReapsBothPlanes(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := threeStepExecution()

	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec[:2]))
	require.NoError(t, cp.Save(samplePosition(), exec))

	require.NoError(t, cp.Revert("run-1", 1))

	// Steps 0 and 1 survive on both planes; step 2's rows are gone from both.
	require.Contains(t, db.store.results, rowKey("run-1", 0))
	require.Contains(t, db.store.results, rowKey("run-1", 1))
	require.NotContains(t, db.store.results, rowKey("run-1", 2), "forward-plane row past the target step is reaped")
	require.Contains(t, db.store.receipts, rowKey("run-1", 1))
	require.NotContains(t, db.store.receipts, rowKey("run-1", 2), "reverse-plane row past the target step is reaped")

	// Load returns only steps 0-1 and step 1's receipt still round-trips.
	_, gotExec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, gotExec, 2)
	require.Equal(t, exec[1].Receipt, gotExec[1].Receipt)
}

// TestDoltCheckpointTerminalReapsBothPlanesWithBranch proves invariant (2): the
// terminal-state merge-and-delete reaps the run branch, and with it both planes'
// per-run rows (srd036-dolt-state-persistence R4.3).
func TestDoltCheckpointTerminalReapsBothPlanesWithBranch(t *testing.T) {
	t.Parallel()
	terminal := func(s State) bool { return s == "Done" }
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", terminal)
	exec := threeStepExecution()

	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec[:2]))

	// Both planes were written on the run branch before the terminal save.
	require.GreaterOrEqual(t, countCalls(db.calls, "REPLACE INTO tool_outputs"), 2)
	require.GreaterOrEqual(t, countCalls(db.calls, "REPLACE INTO receipts"), 2)

	terminalPos := samplePosition()
	terminalPos.CurrentState = "Done"
	require.NoError(t, cp.Save(terminalPos, exec))

	require.Equal(t, 1, countCalls(db.calls, "DOLT_MERGE"), "terminal save merges to main")
	require.Equal(t, 1, countCalls(db.calls, "DOLT_BRANCH('-d'"), "run branch deleted after merge")
	require.False(t, db.branches["run-1"], "run branch and both of its planes are reaped")
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
