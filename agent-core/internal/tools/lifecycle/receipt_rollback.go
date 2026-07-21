// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
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
// most one of failure/skipped is set; otherwise the entry was reversed.
type entryOutcome struct {
	line    string
	skipped bool
	failure *UndoFailure
}

// rollbackViaReceipts reverts the run's persisted DB state to the target step,
// then walks the entries after the target in reverse, reversing each tool's
// external effect through its receipt-driven Undo. DB revert and external
// reversal are distinct: the engine/lifecycle never parses a receipt; only the
// originating tool (rebuilt via core.Reverser) decodes it (srd036 R6; #44).
//
// A failed Undo does not stop the walk (remaining entries are still attempted)
// but yields a *PartialRollbackError so the caller reports CommandError rather
// than a clean rollback (srd026 R3.7, R6.4). The returned summary always carries
// the per-entry report and a reversed/skipped/failed tally (srd026 R3.8).
func rollbackViaReceipts(opts rollbackViaReceiptsOptions) (string, error) {
	targetStep, err := resolveTargetStep(opts.Execution, opts.TargetIteration)
	if err != nil {
		return "", err
	}
	if err := opts.Reverter.Revert(opts.RunID, targetStep); err != nil {
		return "", fmt.Errorf("revert run %q to step %d: %w", opts.RunID, targetStep, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "rolled back run %s to iteration %d (step %d)\n", opts.RunID, opts.TargetIteration, targetStep)
	reversed, skipped := 0, 0
	var failures []UndoFailure
	for step := len(opts.Execution) - 1; step > targetStep; step-- {
		outcome := undoEntry(opts.Registry, opts.Tracer, step, opts.Execution[step])
		b.WriteString(outcome.line)
		switch {
		case outcome.failure != nil:
			failures = append(failures, *outcome.failure)
		case outcome.skipped:
			skipped++
		default:
			reversed++
		}
	}
	fmt.Fprintf(&b, "reversed %d, skipped %d, failed %d\n", reversed, skipped, len(failures))
	if len(failures) > 0 {
		return b.String(), &PartialRollbackError{
			RunID:      opts.RunID,
			TargetStep: targetStep,
			Reverted:   reversed,
			Failures:   failures,
		}
	}
	return b.String(), nil
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
// receipt. Entries whose tool is unregistered, does not implement core.Reverser,
// or carries no receipt are skipped and logged as irreversible. An entry whose
// Undo runs but returns CommandError is a failure, not a skip (srd026 R3.7).
func undoEntry(registry core.CommandResolver, tracer tracing.Tracer, step int, entry core.Entry) entryOutcome {
	if registry == nil {
		return skipOutcome(tracer, step, entry, "no registry")
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
	if res.Signal == core.CommandError || res.Err != nil {
		detail := res.Output
		if detail == "" && res.Err != nil {
			detail = res.Err.Error()
		}
		if tracer != nil {
			tracer.Event("rollback.entry_undo_failed",
				attribute.Int("step", step),
				attribute.String("command", entry.CommandName),
				attribute.String("detail", detail),
			)
		}
		return entryOutcome{
			line:    fmt.Sprintf("  step=%d %s: undo failed: %s\n", step, entry.CommandName, res.Output),
			failure: &UndoFailure{Step: step, CommandName: entry.CommandName, Detail: detail},
		}
	}
	return entryOutcome{line: fmt.Sprintf("  step=%d %s: %s\n", step, entry.CommandName, res.Output)}
}

func skipOutcome(tracer tracing.Tracer, step int, entry core.Entry, reason string) entryOutcome {
	if tracer != nil {
		tracer.Event("rollback.entry_skipped",
			attribute.Int("step", step),
			attribute.String("command", entry.CommandName),
			attribute.String("reason", reason),
		)
	}
	return entryOutcome{
		line:    fmt.Sprintf("  step=%d %s: skipped (%s)\n", step, entry.CommandName, reason),
		skipped: true,
	}
}
