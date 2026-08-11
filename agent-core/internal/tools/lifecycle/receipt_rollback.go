// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// rollbackViaReceiptsOptions configures a two-part rollback: a git-style DB
// Revert followed by a reverse receipt walk that reverses external effects.
type rollbackViaReceiptsOptions struct {
	Reverter        core.CheckpointReverter
	Registry        core.CommandResolver
	Tracer          tracing.Tracer
	RunID           string
	Execution       core.Execution
	TargetIteration int
}

// UndoFailure records one entry whose receipt-walk Undo failed during rollback,
// so an operator can see which external effect was not reversed (srd026 R6.3).
type UndoFailure struct {
	Step        int
	CommandName string
	Detail      string
}

// PendingCompensation records an entry whose declared Undo requires an
// operator or a later machine action. It is a successful classification of a
// compensatable effect, not a receipt-walk failure.
type PendingCompensation struct {
	Step        int      `json:"step"`
	CommandName string   `json:"command"`
	Description string   `json:"description"`
	Requires    []string `json:"requires"`
}

// PartialRollbackError reports that the DB Revert succeeded but one or more
// receipt-walk Undo calls failed, so external effects are only partly reversed.
// It carries the reverted count and each failed entry so callers do not mistake
// a partial reversal for a clean rollback (srd026 R3.7, R6.2, R6.3, R6.4).
type PartialRollbackError struct {
	RunID      string
	TargetStep int
	Reverted   int
	Failures   []UndoFailure
}

func (e *PartialRollbackError) Error() string {
	names := make([]string, len(e.Failures))
	for i, f := range e.Failures {
		names[i] = fmt.Sprintf("step=%d %s", f.Step, f.CommandName)
	}
	return fmt.Sprintf("rollback of run %s to step %d partially failed: %d reversed, %d receipt-walk Undo failure(s): %s",
		e.RunID, e.TargetStep, e.Reverted, len(e.Failures), strings.Join(names, ", "))
}

// entryOutcome is the result of attempting to reverse one persisted step: at
// most one of failure, skipped, or compensation is set; otherwise it reversed.
type entryOutcome struct {
	line         string
	skipped      bool
	failure      *UndoFailure
	compensation *PendingCompensation
}

// rollbackReport is the structured result of a receipt walk, projected into the
// checkpoint_rollback tool's declared output schema (srd026 R3.8): run identity,
// target step, reverted count, skipped irreversible entries, and pending
// compensations. Detail carries the per-entry report for tracing and operators.
type rollbackReport struct {
	RunID               string
	TargetStep          int
	Reverted            int
	Skipped             []string
	PendingCompensation []PendingCompensation
	Detail              string
}

// rollbackViaReceipts reverts the run's persisted DB state to the target step,
// then walks the entries after the target in reverse, reversing each tool's
// external effect through its receipt-driven Undo. DB revert and external
// reversal are distinct: the engine/lifecycle never parses a receipt; only the
// originating tool (rebuilt via core.Reverser) decodes it (srd036 R6; #44).
//
// A failed Undo does not stop the walk (remaining entries are still attempted)
// but yields a *PartialRollbackError so the caller reports CommandError rather
// than a clean rollback (srd026 R3.7, R6.4). Declared manual compensation is a
// separate successful outcome, not an Undo failure.
func rollbackViaReceipts(opts rollbackViaReceiptsOptions) (rollbackReport, error) {
	targetStep, err := resolveTargetStep(opts.Execution, opts.TargetIteration)
	if err != nil {
		return rollbackReport{}, err
	}
	if err := opts.Reverter.Revert(opts.RunID, targetStep); err != nil {
		return rollbackReport{}, fmt.Errorf("revert run %q to step %d: %w", opts.RunID, targetStep, err)
	}
	report, failures := walkRollbackEntries(opts, targetStep)
	if len(failures) == 0 {
		return report, nil
	}
	return report, &PartialRollbackError{
		RunID: opts.RunID, TargetStep: targetStep,
		Reverted: report.Reverted, Failures: failures,
	}
}

func walkRollbackEntries(opts rollbackViaReceiptsOptions, targetStep int) (rollbackReport, []UndoFailure) {
	var b strings.Builder
	fmt.Fprintf(&b, "rolled back run %s to iteration %d (step %d)\n", opts.RunID, opts.TargetIteration, targetStep)
	report := rollbackReport{
		RunID: opts.RunID, TargetStep: targetStep,
		Skipped: []string{}, PendingCompensation: []PendingCompensation{},
	}
	var failures []UndoFailure
	for step := len(opts.Execution) - 1; step > targetStep; step-- {
		entry := opts.Execution[step]
		outcome := undoEntry(opts.Registry, opts.Tracer, step, entry)
		b.WriteString(outcome.line)
		applyEntryOutcome(&report, &failures, entry, outcome)
	}
	fmt.Fprintf(&b, "reversed %d, pending compensation %d, skipped %d, failed %d\n",
		report.Reverted, len(report.PendingCompensation), len(report.Skipped), len(failures))
	report.Detail = b.String()
	return report, failures
}

func applyEntryOutcome(
	report *rollbackReport,
	failures *[]UndoFailure,
	entry core.Entry,
	outcome entryOutcome,
) {
	switch {
	case outcome.failure != nil:
		*failures = append(*failures, *outcome.failure)
	case outcome.compensation != nil:
		report.PendingCompensation = append(report.PendingCompensation, *outcome.compensation)
	case outcome.skipped:
		report.Skipped = append(report.Skipped, entry.CommandName)
	default:
		report.Reverted++
	}
}

// rollbackOutput projects a rollback report into the checkpoint_rollback tool's
// declared JSON output schema {run, target_step, reverted_entries,
// skipped_irreversible, pending_compensation} (srd026 R3.8). On partial failure
// it adds failed entries and an error so an operator can choose recovery.
func rollbackOutput(report rollbackReport, partial *PartialRollbackError) string {
	skipped := report.Skipped
	if skipped == nil {
		skipped = []string{}
	}
	m := map[string]any{
		"run":                  report.RunID,
		"target_step":          report.TargetStep,
		"reverted_entries":     report.Reverted,
		"skipped_irreversible": skipped,
		"pending_compensation": report.PendingCompensation,
		"detail":               report.Detail,
	}
	if partial != nil {
		failed := make([]map[string]any, len(partial.Failures))
		for i, f := range partial.Failures {
			failed[i] = map[string]any{"step": f.Step, "command": f.CommandName, "detail": f.Detail}
		}
		m["failed_entries"] = failed
		m["error"] = partial.Error()
	}
	out, err := json.Marshal(m)
	if err != nil {
		return report.Detail
	}
	return string(out)
}

// resolveTargetStep maps a target iteration to its step index in the ordered
// Execution log. The step index is the DB Revert target; entries after it are
// reversed by the receipt walk.
func resolveTargetStep(execution core.Execution, targetIteration int) (int, error) {
	for step := len(execution) - 1; step >= 0; step-- {
		if execution[step].Iteration == targetIteration {
			return step, nil
		}
	}
	return 0, fmt.Errorf("target iteration %d not found in execution log", targetIteration)
}

// undoEntry reverses one persisted step's external effect. It rebuilds a fresh,
// undo-only command through core.Reverser and drives it from the entry's opaque
// receipt. A registered declaration identifies irreversible tools; unavailable
// builders and receipts retain compatibility skips pending GH-1584.
// CompensationRequired becomes operator work, while CommandError is a failure.
func undoEntry(registry core.CommandResolver, tracer tracing.Tracer, step int, entry core.Entry) entryOutcome {
	if registry == nil {
		return skipOutcome(tracer, step, entry, "no registry")
	}
	policy, declared := rollbackPolicy(registry, entry.CommandName)
	if declared && policy.Classification == "irreversible" {
		return skipOutcome(tracer, step, entry, "irreversible by declaration")
	}
	builder, ok := registry.Resolve(entry.CommandName)
	if !ok {
		return skipOutcome(tracer, step, entry, "no builder registered")
	}
	reverser, ok := builder.(core.Reverser)
	if !ok {
		return skipOutcome(tracer, step, entry, "irreversible")
	}
	if entry.Receipt == "" {
		return skipOutcome(tracer, step, entry, "no receipt")
	}
	res := reverser.BuildReverser().Undo(core.Result{
		Receipt:     entry.Receipt,
		Output:      entry.Result.Output,
		CommandName: entry.CommandName,
	})
	if res.Signal == core.CompensationRequired {
		return compensationOutcome(tracer, step, entry, policy, res)
	}
	if res.Signal == core.CommandError || res.Err != nil {
		return undoFailureOutcome(tracer, step, entry, res)
	}
	// GH-1377: surface every reversed entry, not just failures/skips, so the
	// compensation dispatches this word fans out — notably REST Undo re-entering
	// the HTTP client — appear on the rollback span instead of being invisible.
	traceRollbackEntry(tracer, "rollback.entry_reversed", step, entry.CommandName,
		attribute.String("detail", res.Output))
	return entryOutcome{line: fmt.Sprintf("  step=%d %s: %s\n", step, entry.CommandName, res.Output)}
}

func rollbackPolicy(registry core.CommandResolver, commandName string) (core.RollbackPolicy, bool) {
	resolver, ok := registry.(core.ToolSpecResolver)
	if !ok {
		return core.RollbackPolicy{}, false
	}
	spec, ok := resolver.SpecByName(commandName)
	if !ok {
		return core.RollbackPolicy{}, false
	}
	return spec.Rollback, true
}

func compensationOutcome(
	tracer tracing.Tracer,
	step int,
	entry core.Entry,
	policy core.RollbackPolicy,
	res core.Result,
) entryOutcome {
	description := policy.Description
	if description == "" {
		description = res.Output
	}
	pending := &PendingCompensation{
		Step: step, CommandName: entry.CommandName,
		Description: description, Requires: append([]string{}, policy.Requires...),
	}
	traceRollbackEntry(tracer, "rollback.entry_compensation_required", step, entry.CommandName,
		attribute.String("detail", description))
	return entryOutcome{
		line: fmt.Sprintf("  step=%d %s: compensation required: %s\n",
			step, entry.CommandName, description),
		compensation: pending,
	}
}

// undoFailureOutcome records a receipt-walk Undo that returned CommandError.
func undoFailureOutcome(tracer tracing.Tracer, step int, entry core.Entry, res core.Result) entryOutcome {
	detail := res.Output
	if detail == "" && res.Err != nil {
		detail = res.Err.Error()
	}
	traceRollbackEntry(tracer, "rollback.entry_undo_failed", step, entry.CommandName,
		attribute.String("detail", detail))
	return entryOutcome{
		line:    fmt.Sprintf("  step=%d %s: undo failed: %s\n", step, entry.CommandName, res.Output),
		failure: &UndoFailure{Step: step, CommandName: entry.CommandName, Detail: detail},
	}
}

func skipOutcome(tracer tracing.Tracer, step int, entry core.Entry, reason string) entryOutcome {
	traceRollbackEntry(tracer, "rollback.entry_skipped", step, entry.CommandName,
		attribute.String("reason", reason))
	return entryOutcome{
		line:    fmt.Sprintf("  step=%d %s: skipped (%s)\n", step, entry.CommandName, reason),
		skipped: true,
	}
}

// traceRollbackEntry emits one rollback receipt-walk event so each compensation
// attempt (reversed, failed, or skipped) is recorded on the rollback span.
func traceRollbackEntry(tracer tracing.Tracer, event string, step int, command string, extra attribute.KeyValue) {
	if tracer == nil {
		return
	}
	tracer.Event(event,
		attribute.Int("step", step),
		attribute.String("command", command),
		extra,
	)
}
