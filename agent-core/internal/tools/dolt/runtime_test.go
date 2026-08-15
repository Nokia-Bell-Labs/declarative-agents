// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

const testDSN = "alice:dsn-secret@tcp(localhost:3306)/ignored"

func TestProvisionCreatesAppliesInOrderAndCommitsOnlyChanges(t *testing.T) {
	t.Parallel()
	cfg := baseConfig(KindProvision)
	cfg.SchemaStatements = []string{
		"CREATE TABLE IF NOT EXISTS tickets (id VARCHAR(64) PRIMARY KEY)",
		"CREATE INDEX idx_ticket ON tickets (id)",
	}

	t.Run("changed", func(t *testing.T) {
		server := &fakeDatabase{rowValues: []interface{}{0}}
		tx := &fakeTransaction{statusChanges: 2, commitHash: "schemahash"}
		target := &fakeDatabase{tx: tx}
		builder := provisionBuilder(t, cfg, &sequenceOpener{databases: []Database{server, target}})

		result := builder.Build(core.Result{Output: `{"parameters":{}}`}).Execute()

		require.Equal(t, SignalDoltProvisioned, result.Signal, result.Output)
		require.Equal(t, []string{cfg.SchemaStatements[0], cfg.SchemaStatements[1], doltStageSQL}, tx.execQueries())
		require.Equal(t, []string{doltStatusSQL, doltCommitSQL}, tx.rowQueries())
		require.Equal(t, "Provision tickets", tx.rowCalls[1].args[0])
		require.Equal(t, 1, tx.commits)
		require.Equal(t, 1, server.execCount("CREATE DATABASE"))
		output := decodeOutput(t, result)
		require.Equal(t, true, output["created"])
		require.Equal(t, true, output["schema_changed"])
		require.Equal(t, "schemahash", output["commit_hash"])
	})

	t.Run("already current", func(t *testing.T) {
		server := &fakeDatabase{rowValues: []interface{}{1}}
		tx := &fakeTransaction{}
		target := &fakeDatabase{tx: tx}
		builder := provisionBuilder(t, cfg, &sequenceOpener{databases: []Database{server, target}})

		result := builder.Build(core.Result{}).Execute()

		require.Equal(t, SignalDoltProvisioned, result.Signal, result.Output)
		require.Equal(t, []string{cfg.SchemaStatements[0], cfg.SchemaStatements[1]}, tx.execQueries())
		require.Equal(t, []string{doltStatusSQL}, tx.rowQueries())
		require.Equal(t, 1, tx.commits)
		require.Zero(t, server.execCount("CREATE DATABASE"))
		output := decodeOutput(t, result)
		require.Equal(t, false, output["created"])
		require.Equal(t, false, output["schema_changed"])
		require.Empty(t, output["commit_hash"])
	})
}

func TestQueryBindsAndReturnsDeterministicallyBoundedRows(t *testing.T) {
	t.Parallel()
	cfg := baseConfig(KindQuery)
	cfg.MaxRows = 2
	rows := &fakeRows{
		columns: []Column{{Name: "id", DatabaseType: "VARCHAR"}, {Name: "n", DatabaseType: "BIGINT"}},
		values:  [][]interface{}{{[]byte("a"), int64(1)}, {[]byte("b"), int64(2)}, {[]byte("c"), int64(3)}},
	}
	db := &fakeDatabase{rows: rows}
	builder := queryBuilder(t, cfg, &sequenceOpener{databases: []Database{db}})

	result := builder.Build(core.Result{Output: `{"tool":"find","parameters":{"id":"safe"}}`}).Execute()

	require.Equal(t, SignalDoltRowsRead, result.Signal, result.Output)
	require.Len(t, db.queryCalls, 1)
	require.Equal(t, cfg.Statement[:strings.Index(cfg.Statement, ":id")]+"?", db.queryCalls[0].query)
	require.Equal(t, []interface{}{"safe"}, db.queryCalls[0].args)
	output := decodeOutput(t, result)
	require.Equal(t, float64(2), output["row_count"])
	require.Equal(t, true, output["truncated"])
	require.Len(t, output["columns"], 2)
	require.Equal(t, "a", output["rows"].([]interface{})[0].(map[string]interface{})["id"])
	require.Empty(t, db.beginCalls)

	cfg.MaxRows, cfg.MaxBytes = 10, 3
	smallRows := &fakeRows{columns: []Column{{Name: "id"}}, values: [][]interface{}{{"too-large"}}}
	small := &fakeDatabase{rows: smallRows}
	result = queryBuilder(t, cfg, &sequenceOpener{databases: []Database{small}}).
		Build(core.Result{Output: `{"parameters":{"id":"safe"}}`}).Execute()
	output = decodeOutput(t, result)
	require.Equal(t, float64(0), output["row_count"])
	require.Equal(t, true, output["truncated"])
}

func TestWriteStagesAndCommitsExactlyOnceWithTrustedMessage(t *testing.T) {
	t.Parallel()
	cfg := baseConfig(KindWrite)
	tx := &fakeTransaction{affected: 1, commitHash: "writehash"}
	db := &fakeDatabase{tx: tx}
	def := runtimeDef(t, "delete_ticket", cfg)
	def.Reversibility.Classification = "compensatable"
	def.Undo = catalog.ToolUndoContract{
		Strategy: "configured_dolt_compensation", Description: "run declared compensation",
		Requires: []string{"compensation_action"},
	}
	factory := WriteFactory(FactoryDeps{Opener: &sequenceOpener{databases: []Database{db}}})
	builder, err := factory(def, map[string]string{cfg.ConnectionRef: testDSN})
	require.NoError(t, err)
	tracer := &recordingTracer{}
	cmd := builder.Build(core.Result{Output: `{"parameters":{"id":"T-7"}}`})
	cmd.(core.TracerAware).SetTracer(tracer)

	result := cmd.Execute()

	require.Equal(t, SignalDoltCommitted, result.Signal, result.Output)
	require.Equal(t, []string{strings.ReplaceAll(cfg.Statement, ":id", "?"), doltStageSQL}, tx.execQueries())
	require.Equal(t, []string{doltCommitSQL}, tx.rowQueries())
	require.Equal(t, "Delete T-7", tx.rowCalls[0].args[0])
	require.Equal(t, 1, tx.commits)
	require.NotEmpty(t, result.Receipt)
	require.NotContains(t, result.Output+result.Receipt, "T-7")
	require.NotContains(t, result.Output+result.Receipt, "dsn-secret")
	require.Equal(t, core.CompensationRequired, cmd.Undo(result).Signal)
	require.NotContains(t, tracer.joined(), "T-7")
	require.NotContains(t, tracer.joined(), "dsn-secret")
	require.Contains(t, tracer.joined(), "tcp://localhost:3306")
}

func TestWriteNoChangeAndFailuresNeverCreateCommit(t *testing.T) {
	t.Parallel()
	cfg := baseConfig(KindWrite)

	t.Run("default zero rows", func(t *testing.T) {
		tx := &fakeTransaction{}
		builder := writeBuilder(t, cfg, &sequenceOpener{databases: []Database{&fakeDatabase{tx: tx}}})
		result := builder.Build(core.Result{Output: `{"parameters":{"id":"none"}}`}).Execute()
		require.Equal(t, SignalDoltNoChange, result.Signal, result.Output)
		require.Equal(t, []string{strings.ReplaceAll(cfg.Statement, ":id", "?")}, tx.execQueries())
		require.Empty(t, tx.rowCalls)
		require.Equal(t, 1, tx.rollbacks)
		require.Equal(t, false, decodeOutput(t, result)["committed"])
	})

	t.Run("configured empty commit", func(t *testing.T) {
		configured := cfg
		configured.CommitOnNoChange = true
		tx := &fakeTransaction{commitHash: "emptyhash"}
		builder := writeBuilder(t, configured, &sequenceOpener{databases: []Database{&fakeDatabase{tx: tx}}})
		result := builder.Build(core.Result{Output: `{"parameters":{"id":"none"}}`}).Execute()
		require.Equal(t, SignalDoltCommitted, result.Signal, result.Output)
		require.Equal(t, []string{doltEmptyCommitSQL}, tx.rowQueries())
		require.Equal(t, 1, tx.commits)
		require.Equal(t, true, decodeOutput(t, result)["committed"])
	})

	for _, stage := range []string{"stage", "commit"} {
		stage := stage
		t.Run(stage+" failure", func(t *testing.T) {
			tx := &fakeTransaction{affected: 1, commitHash: "unused", failOn: stage}
			builder := writeBuilder(t, cfg, &sequenceOpener{databases: []Database{&fakeDatabase{tx: tx}}})
			result := builder.Build(core.Result{Output: `{"parameters":{"id":"T-8"}}`}).Execute()
			require.Equal(t, core.CommandError, result.Signal, result.Output)
			require.Equal(t, stage, decodeOutput(t, result)["failure_stage"])
			require.Equal(t, 1, tx.rollbacks)
			require.Zero(t, tx.commits)
		})
	}
}

func TestRuntimeRejectsAuthorityAndCancellationBeforeOpening(t *testing.T) {
	t.Parallel()
	cfg := baseConfig(KindQuery)
	opener := &sequenceOpener{}
	builder := queryBuilder(t, cfg, opener)
	result := builder.Build(core.Result{
		Output: `{"tool":"find","parameters":{"id":"ok"},"sql":"SELECT secret"}`,
	}).Execute()
	require.Equal(t, core.CommandError, result.Signal, result.Output)
	require.Equal(t, "parameter_validation", decodeOutput(t, result)["failure_stage"])
	require.Zero(t, opener.opens)
	require.NotContains(t, result.Output, "SELECT secret")

	resolver := testConnectionResolverFunc(func(ctx context.Context, _ string, _ map[string]string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	factory := QueryFactory(FactoryDeps{Opener: opener, Connections: resolver})
	built, err := factory(runtimeDef(t, "cancelled", cfg), nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = built.Build(core.Result{Output: `{"parameters":{"id":"ok"}}`}).(core.ContextCommand).ExecuteContext(ctx)
	require.Equal(t, "cancellation", decodeOutput(t, result)["failure_stage"])
	require.Zero(t, opener.opens)
}

func TestFactoriesStrictlySeparateOperationKinds(t *testing.T) {
	t.Parallel()
	cfg := baseConfig(KindWrite)
	_, err := QueryFactory(FactoryDeps{})(runtimeDef(t, "wrong_kind", cfg), nil)
	require.ErrorContains(t, err, `requires kind "query", got "write"`)

	cfg = baseConfig(KindQuery)
	def := runtimeDef(t, "unknown_field", cfg)
	def.Config["unexpected"] = "refused"
	_, err = QueryFactory(FactoryDeps{})(def, nil)
	require.ErrorContains(t, err, `unknown field "unexpected"`)

	builder := queryBuilder(t, cfg, &sequenceOpener{databases: []Database{&fakeDatabase{rows: &fakeRows{}}}})
	cmd := builder.Build(core.Result{Output: `{"parameters":{"id":"ok"}}`})
	_, overrides := cmd.(core.SpanOverride)
	require.False(t, overrides)
	require.Implements(t, (*core.TracerAware)(nil), cmd)
}

func provisionBuilder(t *testing.T, cfg OperationConfig, opener DatabaseOpener) ProvisionBuilder {
	t.Helper()
	b, err := ProvisionFactory(FactoryDeps{Opener: opener})(
		runtimeDef(t, "provision", cfg), map[string]string{cfg.ConnectionRef: testDSN},
	)
	require.NoError(t, err)
	return b.(ProvisionBuilder)
}

func queryBuilder(t *testing.T, cfg OperationConfig, opener DatabaseOpener) QueryBuilder {
	t.Helper()
	b, err := QueryFactory(FactoryDeps{Opener: opener})(
		runtimeDef(t, "query", cfg), map[string]string{cfg.ConnectionRef: testDSN},
	)
	require.NoError(t, err)
	return b.(QueryBuilder)
}

func writeBuilder(t *testing.T, cfg OperationConfig, opener DatabaseOpener) WriteBuilder {
	t.Helper()
	b, err := WriteFactory(FactoryDeps{Opener: opener})(
		runtimeDef(t, "write", cfg), map[string]string{cfg.ConnectionRef: testDSN},
	)
	require.NoError(t, err)
	return b.(WriteBuilder)
}

func runtimeDef(t *testing.T, name string, cfg OperationConfig) catalog.ToolDef {
	t.Helper()
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	values := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(data, &values))
	return catalog.ToolDef{Name: name, Config: values}
}

func decodeOutput(t *testing.T, result core.Result) map[string]interface{} {
	t.Helper()
	output := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output
}

type call struct {
	query string
	args  []interface{}
}

type testConnectionResolverFunc func(context.Context, string, map[string]string) (string, error)

func (f testConnectionResolverFunc) ResolveConnection(
	ctx context.Context,
	ref string,
	vars map[string]string,
) (string, error) {
	return f(ctx, ref, vars)
}

type sequenceOpener struct {
	mu        sync.Mutex
	databases []Database
	opens     int
}

func (o *sequenceOpener) Open(string) (Database, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opens++
	if len(o.databases) == 0 {
		return nil, errors.New("unexpected open with secret details")
	}
	db := o.databases[0]
	o.databases = o.databases[1:]
	return db, nil
}

type fakeDatabase struct {
	tx         *fakeTransaction
	rows       Rows
	rowValues  []interface{}
	execCalls  []call
	queryCalls []call
	beginCalls []struct{}
}

func (*fakeDatabase) PingContext(context.Context) error { return nil }
func (d *fakeDatabase) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	d.execCalls = append(d.execCalls, call{query: query, args: args})
	return fakeResult(0), nil
}
func (d *fakeDatabase) QueryRowContext(_ context.Context, query string, args ...any) Scanner {
	d.queryCalls = append(d.queryCalls, call{query: query, args: args})
	return fakeScanner{values: d.rowValues}
}
func (d *fakeDatabase) QueryContext(_ context.Context, query string, args ...any) (Rows, error) {
	d.queryCalls = append(d.queryCalls, call{query: query, args: args})
	return d.rows, nil
}
func (d *fakeDatabase) BeginTx(context.Context) (Transaction, error) {
	d.beginCalls = append(d.beginCalls, struct{}{})
	return d.tx, nil
}
func (*fakeDatabase) Close() error { return nil }
func (d *fakeDatabase) execCount(prefix string) int {
	count := 0
	for _, call := range d.execCalls {
		if strings.HasPrefix(call.query, prefix) {
			count++
		}
	}
	return count
}

type fakeTransaction struct {
	execCalls, rowCalls []call
	affected            int64
	statusChanges       int
	commitHash, failOn  string
	commits, rollbacks  int
}

func (t *fakeTransaction) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	t.execCalls = append(t.execCalls, call{query: query, args: args})
	if t.failOn == "stage" && query == doltStageSQL {
		return nil, errors.New("stage failed and contained raw SQL")
	}
	return fakeResult(t.affected), nil
}
func (t *fakeTransaction) QueryRowContext(_ context.Context, query string, args ...any) Scanner {
	t.rowCalls = append(t.rowCalls, call{query: query, args: args})
	if query == doltStatusSQL {
		return fakeScanner{values: []interface{}{t.statusChanges}}
	}
	if t.failOn == "commit" {
		return fakeScanner{err: errors.New("commit failed with secret message")}
	}
	return fakeScanner{values: []interface{}{t.commitHash}}
}
func (t *fakeTransaction) Commit() error   { t.commits++; return nil }
func (t *fakeTransaction) Rollback() error { t.rollbacks++; return nil }
func (t *fakeTransaction) execQueries() []string {
	out := make([]string, len(t.execCalls))
	for i := range t.execCalls {
		out[i] = t.execCalls[i].query
	}
	return out
}
func (t *fakeTransaction) rowQueries() []string {
	out := make([]string, len(t.rowCalls))
	for i := range t.rowCalls {
		out[i] = t.rowCalls[i].query
	}
	return out
}

type fakeResult int64

func (r fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeResult) RowsAffected() (int64, error) { return int64(r), nil }

type fakeScanner struct {
	values []interface{}
	err    error
}

func (s fakeScanner) Scan(dest ...any) error {
	if s.err != nil {
		return s.err
	}
	for i := range dest {
		switch pointer := dest[i].(type) {
		case *int:
			*pointer = s.values[i].(int)
		case *string:
			*pointer = s.values[i].(string)
		case *interface{}:
			*pointer = s.values[i]
		}
	}
	return nil
}

type fakeRows struct {
	columns []Column
	values  [][]interface{}
	index   int
}

func (r *fakeRows) Columns() ([]Column, error) { return r.columns, nil }
func (r *fakeRows) Next() bool                 { return r.index < len(r.values) }
func (r *fakeRows) Scan(dest ...any) error {
	for i := range dest {
		*(dest[i].(*interface{})) = r.values[r.index][i]
	}
	r.index++
	return nil
}
func (*fakeRows) Err() error   { return nil }
func (*fakeRows) Close() error { return nil }

type recordingTracer struct {
	attrs []attribute.KeyValue
}

func (r *recordingTracer) Push(string, ...attribute.KeyValue) (tracing.Tracer, func()) {
	return r, func() {}
}
func (*recordingTracer) Event(string, ...attribute.KeyValue) {}
func (r *recordingTracer) SetAttributes(attrs ...attribute.KeyValue) {
	r.attrs = append(r.attrs, attrs...)
}
func (*recordingTracer) RecordError(error)        {}
func (*recordingTracer) Context() context.Context { return context.Background() }
func (r *recordingTracer) joined() string {
	var values []string
	for _, attr := range r.attrs {
		values = append(values, string(attr.Key)+"="+attr.Value.String())
	}
	return strings.Join(values, " ")
}
