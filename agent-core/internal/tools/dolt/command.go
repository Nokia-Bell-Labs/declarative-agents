// Copyright (c) 2026 Nokia. All rights reserved.

package dolt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

const SignalDoltProvisioned core.Signal = "DoltProvisioned"
const SignalDoltRowsRead core.Signal = "DoltRowsRead"
const SignalDoltCommitted core.Signal = "DoltCommitted"
const SignalDoltNoChange core.Signal = "DoltNoChange"

type command struct {
	builder  builderConfig
	params   map[string]interface{}
	buildErr error
	tracer   tracing.Tracer
	identity DatabaseIdentity
	rows     int64
	commit   string
}

func newCommand(cfg builderConfig, params map[string]interface{}, buildErr error) *command {
	return &command{builder: cfg, params: params, buildErr: buildErr}
}
func (c *command) Name() string                    { return c.builder.toolName }
func (c *command) SetTracer(tracer tracing.Tracer) { c.tracer = tracer }
func (c *command) Execute() core.Result            { return c.ExecuteContext(context.Background()) }

var (
	_ core.Command        = (*command)(nil)
	_ core.ContextCommand = (*command)(nil)
	_ core.TracerAware    = (*command)(nil)
)

func (c *command) ExecuteContext(parent context.Context) core.Result {
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, c.builder.config.TimeoutDuration)
	defer cancel()
	c.traceBase()

	query, args, message, err := c.prepareInput()
	if err != nil {
		return c.finish(start, c.failure(ctx, "parameter_validation", err))
	}
	dsn, err := c.builder.resolver.ResolveConnection(
		ctx, c.builder.config.ConnectionRef, c.builder.vars,
	)
	if err != nil {
		return c.finish(start, c.failure(ctx, "connection_resolution", err))
	}
	serverDSN, databaseDSN, identity, err := operationDSNs(dsn, c.builder.config.Database)
	if err != nil {
		return c.finish(start, c.failure(ctx, "connection_resolution", err))
	}
	c.identity = identity
	c.traceIdentity()

	var result core.Result
	switch c.builder.config.Kind {
	case KindProvision:
		result = c.provision(ctx, serverDSN, databaseDSN)
	case KindQuery:
		result = c.query(ctx, databaseDSN, query, args)
	case KindWrite:
		result = c.write(ctx, databaseDSN, query, args, message)
	default:
		result = c.failure(ctx, "config_validation", errors.New("unsupported operation kind"))
	}
	return c.finish(start, result)
}
func (c *command) prepareInput() (string, []interface{}, string, error) {
	if c.buildErr != nil {
		return "", nil, "", c.buildErr
	}
	if c.builder.config.Kind == KindProvision {
		return "", nil, "", c.builder.config.Parameters.Validate(c.params)
	}
	query, args, err := c.builder.config.Bind(c.params)
	if err != nil {
		return "", nil, "", err
	}
	if c.builder.config.Kind != KindWrite {
		return query, args, "", nil
	}
	message, err := c.builder.config.RenderCommitMessage(c.params)
	return query, args, message, err
}
func (c *command) provision(ctx context.Context, serverDSN, databaseDSN string) core.Result {
	server, err := c.open(ctx, serverDSN)
	if err != nil {
		return c.failure(ctx, "connection_resolution", err)
	}
	defer func() { _ = server.Close() }()

	var count int
	if err := server.QueryRowContext(
		ctx, databaseExistsSQL, c.builder.config.Database,
	).Scan(&count); err != nil {
		return c.failure(ctx, "database_create", err)
	}
	created := count == 0
	if created {
		create := "CREATE DATABASE IF NOT EXISTS `" + c.builder.config.Database + "`"
		if _, err := server.ExecContext(ctx, create); err != nil {
			return c.failure(ctx, "database_create", err)
		}
	}

	db, err := c.open(ctx, databaseDSN)
	if err != nil {
		return c.failure(ctx, "connection_resolution", err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx)
	if err != nil {
		return c.failure(ctx, "schema_apply", err)
	}
	defer func() { _ = tx.Rollback() }()
	return c.applyAndCommitSchema(ctx, tx, created)
}
func (c *command) applyAndCommitSchema(ctx context.Context, tx Transaction, created bool) core.Result {
	for _, statement := range c.builder.config.SchemaStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return c.failure(ctx, "schema_apply", err)
		}
	}
	var changes int
	if err := tx.QueryRowContext(ctx, doltStatusSQL).Scan(&changes); err != nil {
		return c.failure(ctx, "schema_apply", err)
	}
	if changes == 0 {
		if err := tx.Commit(); err != nil {
			return c.failure(ctx, "schema_apply", err)
		}
		return c.provisionResult(created, false, "", "schema already current")
	}
	if _, err := tx.ExecContext(ctx, doltStageSQL); err != nil {
		return c.failure(ctx, "stage", err)
	}
	message, err := c.builder.config.RenderCommitMessage(map[string]interface{}{})
	if err != nil {
		return c.failure(ctx, "parameter_validation", err)
	}
	hash, err := commitDolt(ctx, tx, doltCommitSQL, message)
	if err != nil {
		return c.failure(ctx, "commit", err)
	}
	if err := tx.Commit(); err != nil {
		return c.failure(ctx, "commit", err)
	}
	c.commit = hash
	return c.provisionResult(created, true, hash, "applied configured schema")
}
func (c *command) provisionResult(created, changed bool, hash, diagnostic string) core.Result {
	output := map[string]interface{}{
		"operation": c.builder.config.Operation, "database": c.builder.config.Database,
		"created": created, "schema_changed": changed, "commit_hash": hash,
		"diagnostics": []string{diagnostic},
	}
	return c.success(SignalDoltProvisioned, output, hash)
}
func (c *command) query(ctx context.Context, dsn, query string, args []interface{}) core.Result {
	start := time.Now()
	db, err := c.open(ctx, dsn)
	if err != nil {
		return c.failure(ctx, "connection_resolution", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return c.failure(ctx, "statement_execute", err)
	}
	columns, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		return c.failure(ctx, "result_scan", err)
	}

	values, truncated, err := c.collectQueryRows(rows, columns)
	if err != nil {
		_ = rows.Close()
		return c.failure(ctx, "result_scan", err)
	}
	if err := rows.Close(); err != nil {
		return c.failure(ctx, "result_scan", err)
	}
	c.rows = int64(len(values))
	output := map[string]interface{}{
		"operation": c.builder.config.Operation, "database": c.builder.config.Database,
		"columns": columns, "rows": values, "row_count": len(values),
		"truncated": truncated, "elapsed": time.Since(start).String(),
	}
	return c.success(SignalDoltRowsRead, output, "")
}
func (c *command) collectQueryRows(rows Rows, columns []Column) ([]map[string]interface{}, bool, error) {
	values := make([]map[string]interface{}, 0, min(c.builder.config.MaxRows, 64))
	encodedBytes := 2
	truncated := false
	for rows.Next() {
		if len(values) >= c.builder.config.MaxRows {
			truncated = true
			break
		}
		dest, scanned := scanDestinations(len(columns))
		if err := rows.Scan(dest...); err != nil {
			return nil, false, err
		}
		row := make(map[string]interface{}, len(columns))
		for i, column := range columns {
			row[column.Name] = normalizeSQLValue(scanned[i])
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return nil, false, err
		}
		separator := 0
		if len(values) > 0 {
			separator = 1
		}
		if encodedBytes+separator+len(encoded) > c.builder.config.MaxBytes {
			truncated = true
			break
		}
		encodedBytes += separator + len(encoded)
		values = append(values, row)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return values, truncated, nil
}
func (c *command) write(ctx context.Context, dsn, query string, args []interface{}, message string) core.Result {
	start := time.Now()
	db, err := c.open(ctx, dsn)
	if err != nil {
		return c.failure(ctx, "connection_resolution", err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx)
	if err != nil {
		return c.failure(ctx, "statement_execute", err)
	}
	defer func() { _ = tx.Rollback() }()
	mutation, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return c.failure(ctx, "statement_execute", err)
	}
	affected, err := mutation.RowsAffected()
	if err != nil {
		return c.failure(ctx, "statement_execute", err)
	}
	c.rows = affected
	if affected == 0 && !c.builder.config.CommitOnNoChange {
		output := c.writeOutput(start, affected, "", false)
		return c.success(SignalDoltNoChange, output, "")
	}
	if _, err := tx.ExecContext(ctx, doltStageSQL); err != nil {
		return c.failure(ctx, "stage", err)
	}
	commitSQL := doltCommitStatement(affected)
	hash, err := commitDolt(ctx, tx, commitSQL, message)
	if err != nil {
		return c.failure(ctx, "commit", err)
	}
	if err := tx.Commit(); err != nil {
		return c.failure(ctx, "commit", err)
	}
	c.commit = hash
	return c.success(SignalDoltCommitted, c.writeOutput(start, affected, hash, true), hash)
}
func doltCommitStatement(affected int64) string {
	if affected == 0 {
		return doltEmptyCommitSQL
	}
	return doltCommitSQL
}
func (c *command) writeOutput(start time.Time, affected int64, hash string, committed bool) map[string]interface{} {
	return map[string]interface{}{
		"operation": c.builder.config.Operation, "database": c.builder.config.Database,
		"rows_affected": affected, "commit_hash": hash, "committed": committed,
		"elapsed": time.Since(start).String(),
	}
}
func (c *command) open(ctx context.Context, dsn string) (Database, error) {
	db, err := c.builder.opener.Open(dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
func commitDolt(ctx context.Context, tx Transaction, statement, message string) (string, error) {
	var hash string
	if err := tx.QueryRowContext(ctx, statement, message).Scan(&hash); err != nil {
		return "", err
	}
	if strings.TrimSpace(hash) == "" {
		return "", errors.New("Dolt commit returned no hash")
	}
	return hash, nil
}
func scanDestinations(count int) ([]interface{}, []interface{}) {
	values := make([]interface{}, count)
	dest := make([]interface{}, count)
	for i := range values {
		dest[i] = &values[i]
	}
	return dest, values
}
func normalizeSQLValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case sql.RawBytes:
		return string(typed)
	default:
		return typed
	}
}
func decodeRuntimeParams(output string) (map[string]interface{}, error) {
	if output == "" {
		return map[string]interface{}{}, nil
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		return map[string]interface{}{}, nil
	}
	if err := ValidateRuntimeInput(envelope); err != nil {
		return nil, err
	}
	if raw, present := envelope["parameters"]; present {
		params, ok := raw.(map[string]interface{})
		if !ok {
			return nil, errors.New("runtime input parameters must be an object")
		}
		return params, nil
	}
	delete(envelope, "tool")
	return envelope, nil
}
func (c *command) success(signal core.Signal, output map[string]interface{}, hash string) core.Result {
	data, err := json.Marshal(output)
	if err != nil {
		return c.failure(context.Background(), "result_scan", err)
	}
	result := core.Result{Signal: signal, CommandName: c.Name(), Output: string(data)}
	if hash != "" && c.compensatable() {
		result.Receipt = undo.EncodeBoundaryReceipt(undo.BoundaryCompensationPayload{
			BoundaryCompensation: undo.BoundaryCompensation{
				Strategy: c.builder.undo.Strategy, Reason: c.builder.undo.Description,
				Requires: append([]string(nil), c.builder.undo.Requires...),
				Data: map[string]interface{}{
					"operation": c.builder.config.Operation, "database": c.builder.config.Database,
					"server": c.identity.Server, "commit_hash": hash,
				},
			},
		})
	}
	return result
}
func (c *command) failure(ctx context.Context, stage string, _ error) core.Result {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		stage = "timeout"
	} else if errors.Is(ctx.Err(), context.Canceled) {
		stage = "cancellation"
	}
	message := "Dolt operation failed during " + stage
	output, _ := json.Marshal(map[string]interface{}{
		"operation": c.builder.config.Operation, "database": c.builder.config.Database,
		"failure_stage": stage, "message": message, "signal": string(core.CommandError),
	})
	err := errors.New(message)
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: string(output), Err: err}
}
func (c *command) finish(start time.Time, result core.Result) core.Result {
	elapsed := time.Since(start)
	result.Cost.Duration = elapsed
	if c.tracer != nil {
		c.tracer.SetAttributes(
			attribute.Int64("dolt.rows", c.rows),
			attribute.Bool("dolt.commit_present", c.commit != ""),
			attribute.String("dolt.commit_hash", c.commit),
			attribute.Int64("dolt.elapsed_ms", elapsed.Milliseconds()),
			attribute.String("dolt.signal", string(result.Signal)),
		)
		if result.Err != nil {
			c.tracer.RecordError(result.Err)
		}
	}
	return result
}
func (c *command) traceBase() {
	if c.tracer == nil {
		return
	}
	c.tracer.SetAttributes(
		attribute.String("dolt.operation", c.builder.config.Operation),
		attribute.String("dolt.kind", string(c.builder.config.Kind)),
		attribute.String("dolt.database", c.builder.config.Database),
	)
}
func (c *command) traceIdentity() {
	if c.tracer != nil {
		c.tracer.SetAttributes(attribute.String("dolt.server", c.identity.Server))
	}
}
func (c *command) compensatable() bool {
	return c.builder.reverse == "compensatable" &&
		c.builder.undo.Strategy != "" && c.builder.undo.Strategy != "noop"
}
func (c *command) Undo(prior core.Result) core.Result {
	if !c.compensatable() || prior.Receipt == "" {
		return core.NoopUndo(c.Name())
	}
	compensation, ok, err := undo.DecodeBoundaryReceipt(prior.Receipt)
	if err != nil {
		return c.failure(context.Background(), "compensation", err)
	}
	if !ok ||
		fmt.Sprint(compensation.Data["operation"]) != c.builder.config.Operation ||
		fmt.Sprint(compensation.Data["database"]) != c.builder.config.Database {
		return c.failure(context.Background(), "compensation", errors.New("receipt identity mismatch"))
	}
	return undo.BoundaryCompensationResult(c.Name(), compensation)
}
