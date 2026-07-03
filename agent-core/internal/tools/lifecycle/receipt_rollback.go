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

// rollbackViaReceipts reverts the run's persisted DB state to the target step,
// then walks the entries after the target in reverse, reversing each tool's
// external effect through its receipt-driven Undo. DB revert and external
// reversal are distinct: the engine/lifecycle never parses a receipt; only the
// originating tool (rebuilt via core.Reverser) decodes it (srd036 R6; #44).
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
	for step := len(opts.Execution) - 1; step > targetStep; step-- {
		b.WriteString(undoEntry(opts.Registry, opts.Tracer, step, opts.Execution[step]))
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
// or carries no receipt are skipped and logged as irreversible.
func undoEntry(registry core.CommandResolver, tracer tracing.Tracer, step int, entry core.Entry) string {
	if registry == nil {
		return skipLine(tracer, step, entry, "no registry")
	}
	builder, ok := registry.Resolve(entry.CommandName)
	if !ok {
		return skipLine(tracer, step, entry, "no builder registered")
	}
	reverser, ok := builder.(core.Reverser)
	if !ok {
		return skipLine(tracer, step, entry, "irreversible")
	}
	if entry.Receipt == "" {
		return skipLine(tracer, step, entry, "no receipt")
	}
	res := reverser.BuildReverser().Undo(core.Result{
		Receipt:     entry.Receipt,
		Output:      entry.Result.Output,
		CommandName: entry.CommandName,
	})
	if res.Signal == core.CommandError || res.Err != nil {
		return fmt.Sprintf("  step=%d %s: undo failed: %s\n", step, entry.CommandName, res.Output)
	}
	return fmt.Sprintf("  step=%d %s: %s\n", step, entry.CommandName, res.Output)
}

func skipLine(tracer tracing.Tracer, step int, entry core.Entry, reason string) string {
	if tracer != nil {
		tracer.Event("rollback.entry_skipped",
			attribute.Int("step", step),
			attribute.String("command", entry.CommandName),
			attribute.String("reason", reason),
		)
	}
	return fmt.Sprintf("  step=%d %s: skipped (%s)\n", step, entry.CommandName, reason)
}
