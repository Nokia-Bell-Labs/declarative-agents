// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrDolt is the base error for the Dolt-backed checkpoint adapter. Connection,
// save, load, and revert failures wrap it so callers can classify by backend
// (srd036-dolt-state-persistence R1.4).
var ErrDolt = errors.New("dolt checkpoint")

// ErrRevertUnresolved reports that a Revert target (run_id, step_index) does not
// resolve to a recorded commit (srd036-dolt-state-persistence R6.5).
var ErrRevertUnresolved = fmt.Errorf("%w: revert target not found", ErrDolt)

// Database is the minimal database/sql-shaped seam the Dolt adapter depends on
// so internal/runtime/core never imports Dolt. sqlDatabase bridges a *sql.DB and
// tests supply a fake (srd036-dolt-state-persistence R1.2, R1.3).
type Database interface {
	Begin() (Transaction, error)
	Exec(query string, args ...any) error
	QueryRow(query string, args ...any) Scanner
	Query(query string, args ...any) (Rows, error)
	Close() error
}

// Transaction is one atomic unit of work: a step's rows and its Dolt commit are
// written together so a partial step is never committed (srd036 R4.4).
type Transaction interface {
	Exec(query string, args ...any) error
	QueryRow(query string, args ...any) Scanner
	Query(query string, args ...any) (Rows, error)
	Commit() error
	Rollback() error
}

// Scanner reads a single row's columns into destinations.
type Scanner interface {
	Scan(dest ...any) error
}

// Rows iterates a multi-row result.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

// DoltCheckpoint implements the Checkpoint port on top of a versioned SQL
// backend reached only through the Database seam. Each run executes on its own
// branch, each loop step is one commit, and terminal runs merge to main
// (srd036-dolt-state-persistence).
type DoltCheckpoint struct {
	db       Database
	runID    string
	terminal func(State) bool
	inited   bool
}

var _ Checkpoint = (*DoltCheckpoint)(nil)

// NewDoltCheckpoint returns an adapter over an already-opened Database seam. The
// terminal predicate, when non-nil, decides which Position current states merge
// the run branch to main; a nil predicate never auto-merges.
func NewDoltCheckpoint(db Database, runID string, terminal func(State) bool) *DoltCheckpoint {
	return &DoltCheckpoint{db: db, runID: runID, terminal: terminal}
}

// OpenDoltCheckpoint opens the Dolt database from a DSN and returns an adapter.
// It uses only the database/sql standard library; the Dolt driver is registered
// at the composition root (cmd/agent) via a blank import, so core never imports
// Dolt types (srd036-dolt-state-persistence R1.3, R1.4).
func OpenDoltCheckpoint(dsn, runID string, terminal func(State) bool) (*DoltCheckpoint, error) {
	db, err := sql.Open("dolt", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: open %q: %v", ErrDolt, dsn, err)
	}
	return NewDoltCheckpoint(newSQLDatabase(db), runID, terminal), nil
}

// Close releases the underlying database handle.
func (d *DoltCheckpoint) Close() error { return d.db.Close() }

// Save appends the current step's rows and creates one Dolt commit per step on
// the run branch, all within a single transaction, then merges to main when the
// Position current state is terminal (srd036-dolt-state-persistence R4).
func (d *DoltCheckpoint) Save(position Position, execution Execution) error {
	if err := d.prepare(); err != nil {
		return err
	}
	step := len(execution) - 1
	sig := Signal("")
	if step >= 0 {
		sig = execution[step].Signal
	}

	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("%w: save: begin: %v", ErrDolt, err)
	}
	if err := writeMachine(tx, d.runID, position); err != nil {
		_ = tx.Rollback()
		return err
	}
	if step >= 0 {
		if err := writeStep(tx, d.runID, step, execution[step]); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Exec(`CALL DOLT_COMMIT('-A', '--allow-empty', '-m', ?)`, commitMessage(step, sig)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("%w: save: commit step %d: %v", ErrDolt, step, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: save: tx commit: %v", ErrDolt, err)
	}

	if d.terminal != nil && d.terminal(position.CurrentState) {
		if err := d.Merge(); err != nil {
			return err
		}
	}
	return nil
}

// Load reconstructs the Position and Execution from the latest commit on the run
// branch, restoring the folded conversation and every opaque receipt. It reports
// ErrNoCheckpoint when the branch or its rows do not exist
// (srd036-dolt-state-persistence R5).
func (d *DoltCheckpoint) Load() (Position, Execution, error) {
	if err := d.db.Exec(`CALL DOLT_CHECKOUT(?)`, d.runID); err != nil {
		return Position{}, nil, ErrNoCheckpoint
	}
	pos, err := loadMachine(d.db, d.runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Position{}, nil, ErrNoCheckpoint
		}
		return Position{}, nil, fmt.Errorf("%w: load: machine: %v", ErrDolt, err)
	}
	exec, err := loadExecution(d.db, d.runID)
	if err != nil {
		return Position{}, nil, fmt.Errorf("%w: load: execution: %v", ErrDolt, err)
	}
	return pos, exec, nil
}

// Merge merges the run branch to main and deletes it, run on a terminal state
// (srd036-dolt-state-persistence R4.3). It is idempotent-safe to call once per
// terminal run.
func (d *DoltCheckpoint) Merge() error {
	if err := d.db.Exec(`CALL DOLT_CHECKOUT('main')`); err != nil {
		return fmt.Errorf("%w: merge: checkout main: %v", ErrDolt, err)
	}
	if err := d.db.Exec(`CALL DOLT_MERGE(?)`, d.runID); err != nil {
		return fmt.Errorf("%w: merge: merge %q: %v", ErrDolt, d.runID, err)
	}
	if err := d.db.Exec(`CALL DOLT_BRANCH('-d', ?)`, d.runID); err != nil {
		return fmt.Errorf("%w: merge: delete branch %q: %v", ErrDolt, d.runID, err)
	}
	d.inited = false
	return nil
}

// Revert resets the run branch to the commit recorded at step_index for git-style
// rollback of DB-persisted state only; file, HTTP, and workspace effects are
// reversed by the lifecycle tool's receipt walk, not here
// (srd036-dolt-state-persistence R6).
func (d *DoltCheckpoint) Revert(runID string, stepIndex int) error {
	if err := d.db.Exec(`CALL DOLT_CHECKOUT(?)`, runID); err != nil {
		return fmt.Errorf("%w: revert: checkout %q: %v", ErrDolt, runID, err)
	}
	var hash string
	row := d.db.QueryRow(
		`SELECT commit_hash FROM dolt_log WHERE message LIKE ? ORDER BY date DESC LIMIT 1`,
		fmt.Sprintf("step %d signal %%", stepIndex),
	)
	if err := row.Scan(&hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: run %q step %d", ErrRevertUnresolved, runID, stepIndex)
		}
		return fmt.Errorf("%w: revert: lookup: %v", ErrDolt, err)
	}
	if err := d.db.Exec(`CALL DOLT_RESET('--hard', ?)`, hash); err != nil {
		return fmt.Errorf("%w: revert: reset %q: %v", ErrDolt, hash, err)
	}
	return nil
}

// prepare checks out (or creates) the run branch and creates the schema once.
func (d *DoltCheckpoint) prepare() error {
	if err := d.ensureBranch(); err != nil {
		return err
	}
	if d.inited {
		return nil
	}
	if err := createSchema(d.db); err != nil {
		return err
	}
	d.inited = true
	return nil
}

// ensureBranch selects the run branch, creating it from the current branch when
// it is absent (srd036-dolt-state-persistence R4.2).
func (d *DoltCheckpoint) ensureBranch() error {
	if err := d.db.Exec(`CALL DOLT_CHECKOUT(?)`, d.runID); err == nil {
		return nil
	}
	if err := d.db.Exec(`CALL DOLT_CHECKOUT('-b', ?)`, d.runID); err != nil {
		return fmt.Errorf("%w: branch %q: %v", ErrDolt, d.runID, err)
	}
	return nil
}

// createSchema creates the generic four-table schema idempotently; it defines no
// per-machine or per-run tables (srd036-dolt-state-persistence R2).
func createSchema(db Database) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS machines (
			run_id VARCHAR(255) PRIMARY KEY,
			current_state VARCHAR(255) NOT NULL,
			last_signal VARCHAR(255) NOT NULL,
			iteration INT NOT NULL,
			tokens_in INT NOT NULL,
			tokens_out INT NOT NULL,
			total_cost DOUBLE NOT NULL,
			conversation LONGTEXT
		)`,
		`CREATE TABLE IF NOT EXISTS transitions (
			run_id VARCHAR(255) NOT NULL,
			step_index INT NOT NULL,
			from_state VARCHAR(255) NOT NULL,
			signal VARCHAR(255) NOT NULL,
			to_state VARCHAR(255) NOT NULL,
			PRIMARY KEY (run_id, step_index)
		)`,
		`CREATE TABLE IF NOT EXISTS execution_steps (
			run_id VARCHAR(255) NOT NULL,
			step_index INT NOT NULL,
			iteration INT NOT NULL,
			ts VARCHAR(64) NOT NULL,
			command_name VARCHAR(255) NOT NULL,
			PRIMARY KEY (run_id, step_index)
		)`,
		`CREATE TABLE IF NOT EXISTS tool_results (
			run_id VARCHAR(255) NOT NULL,
			step_index INT NOT NULL,
			signal VARCHAR(255) NOT NULL,
			output LONGTEXT,
			error LONGTEXT,
			cost_duration BIGINT NOT NULL,
			cost_tokens_in INT NOT NULL,
			cost_tokens_out INT NOT NULL,
			cost_dollars DOUBLE NOT NULL,
			receipt LONGTEXT,
			PRIMARY KEY (run_id, step_index)
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s); err != nil {
			return fmt.Errorf("%w: schema: %v", ErrDolt, err)
		}
	}
	return nil
}

// writeMachine upserts the resumable Position row keyed by run_id.
func writeMachine(tx Transaction, runID string, p Position) error {
	if err := tx.Exec(
		`REPLACE INTO machines
			(run_id, current_state, last_signal, iteration, tokens_in, tokens_out, total_cost, conversation)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, string(p.CurrentState), string(p.LastSignal),
		p.Snapshot.Iteration, p.Snapshot.TokensIn, p.Snapshot.TokensOut, p.Snapshot.TotalCost,
		nullString(string(p.Snapshot.Conversation)),
	); err != nil {
		return fmt.Errorf("%w: save: machine: %v", ErrDolt, err)
	}
	return nil
}

// writeStep appends one Execution entry across transitions, execution_steps, and
// tool_results, keyed by (run_id, step_index) for idempotent retry
// (srd036-dolt-state-persistence R4.1, R4.4).
func writeStep(tx Transaction, runID string, step int, e Entry) error {
	if err := tx.Exec(
		`REPLACE INTO transitions (run_id, step_index, from_state, signal, to_state) VALUES (?, ?, ?, ?, ?)`,
		runID, step, string(e.FromState), string(e.Signal), string(e.ToState),
	); err != nil {
		return fmt.Errorf("%w: save: transition: %v", ErrDolt, err)
	}
	if err := tx.Exec(
		`REPLACE INTO execution_steps (run_id, step_index, iteration, ts, command_name) VALUES (?, ?, ?, ?, ?)`,
		runID, step, e.Iteration, formatTS(e.Timestamp), e.CommandName,
	); err != nil {
		return fmt.Errorf("%w: save: step: %v", ErrDolt, err)
	}
	if err := tx.Exec(
		`REPLACE INTO tool_results
			(run_id, step_index, signal, output, error, cost_duration, cost_tokens_in, cost_tokens_out, cost_dollars, receipt)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, step, string(e.Result.Signal),
		nullString(e.Result.Output), nullString(e.Result.Error),
		int64(e.Result.Cost.Duration), e.Result.Cost.TokensIn, e.Result.Cost.TokensOut, e.Result.Cost.Dollars,
		nullString(e.Receipt),
	); err != nil {
		return fmt.Errorf("%w: save: result: %v", ErrDolt, err)
	}
	return nil
}

// loadMachine reads the Position row, returning sql.ErrNoRows when absent.
func loadMachine(db Database, runID string) (Position, error) {
	var (
		state, signal string
		iteration     int
		tokensIn      int
		tokensOut     int
		totalCost     float64
		conversation  sql.NullString
	)
	err := db.QueryRow(
		`SELECT current_state, last_signal, iteration, tokens_in, tokens_out, total_cost, conversation
			FROM machines WHERE run_id = ?`, runID,
	).Scan(&state, &signal, &iteration, &tokensIn, &tokensOut, &totalCost, &conversation)
	if err != nil {
		return Position{}, err
	}
	pos := Position{
		CurrentState: State(state),
		LastSignal:   Signal(signal),
		Snapshot: AgentSnapshot{
			State:     State(state),
			Signal:    Signal(signal),
			Iteration: iteration,
			TokensIn:  tokensIn,
			TokensOut: tokensOut,
			TotalCost: totalCost,
		},
	}
	if conversation.Valid && conversation.String != "" {
		pos.Snapshot.Conversation = []byte(conversation.String)
	}
	return pos, nil
}

// loadExecution reconstructs the ordered Execution, restoring each entry's
// opaque receipt (srd036-dolt-state-persistence R5.2).
func loadExecution(db Database, runID string) (Execution, error) {
	rows, err := db.Query(
		`SELECT es.step_index, es.iteration, es.ts, es.command_name,
			t.from_state, t.to_state, t.signal,
			r.signal, r.output, r.error, r.cost_duration, r.cost_tokens_in, r.cost_tokens_out, r.cost_dollars, r.receipt
			FROM execution_steps es
			JOIN transitions t ON t.run_id = es.run_id AND t.step_index = es.step_index
			JOIN tool_results r ON r.run_id = es.run_id AND r.step_index = es.step_index
			WHERE es.run_id = ?
			ORDER BY es.step_index`, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execution Execution
	for rows.Next() {
		var (
			stepIndex, iteration                  int
			ts, commandName                       string
			fromState, toState, signal, resSignal string
			output, errStr, receipt               sql.NullString
			costDuration                          int64
			costTokensIn, costTokensOut           int
			costDollars                           float64
		)
		if err := rows.Scan(
			&stepIndex, &iteration, &ts, &commandName,
			&fromState, &toState, &signal,
			&resSignal, &output, &errStr, &costDuration, &costTokensIn, &costTokensOut, &costDollars, &receipt,
		); err != nil {
			return nil, err
		}
		execution = append(execution, Entry{
			Iteration:   iteration,
			Timestamp:   parseTS(ts),
			CommandName: commandName,
			FromState:   State(fromState),
			ToState:     State(toState),
			Signal:      Signal(signal),
			Result: ResultDigest{
				Signal: Signal(resSignal),
				Output: output.String,
				Error:  errStr.String,
				Cost: Cost{
					Duration:  time.Duration(costDuration),
					TokensIn:  costTokensIn,
					TokensOut: costTokensOut,
					Dollars:   costDollars,
				},
			},
			Receipt: receipt.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return execution, nil
}

// commitMessage encodes the step index and signal as Dolt commit metadata so
// Revert can resolve a step to its commit (srd036-dolt-state-persistence R4.1).
func commitMessage(step int, sig Signal) string {
	if step < 0 {
		return "step init signal Seed"
	}
	return fmt.Sprintf("step %d signal %s", step, sig)
}

// nullString maps an empty string to SQL NULL so absent values (for example a
// read-only tool's receipt) store NULL and restore empty
// (srd036-dolt-state-persistence R3.4).
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func formatTS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
