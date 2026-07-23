// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func cmdStateExecution() Execution {
	return Execution{
		{Iteration: 1, CommandName: "embed_query", Label: "query_embedding", Result: commandStateDigest("vec-0"), Receipt: `{"r":0}`},
		{Iteration: 2, CommandName: "record_event", Label: "audit", Result: commandStateDigest("logged"), Receipt: `{"r":1}`},
		{Iteration: 3, CommandName: "embed_query_retry", Label: "query_embedding", Result: commandStateDigest("vec-2"), Receipt: `{"r":2}`},
	}
}

func commandStateDigest(output string) ResultDigest {
	return ResultDigest{
		Output:           output,
		RedactionVersion: OutputRedactionVersion1,
		RedactionStatus:  OutputRedactionApplied,
	}
}

func TestCommandStateViewLookupHit(t *testing.T) {
	t.Parallel()
	view := NewCommandStateView(cmdStateExecution())

	out, ok := view.Lookup("audit")
	require.True(t, ok)
	require.Equal(t, "logged", out)
}

func TestCommandStateViewLookupMiss(t *testing.T) {
	t.Parallel()
	view := NewCommandStateView(cmdStateExecution())

	// A miss returns ok=false and an empty string, never an error, so the caller
	// raises its own typed unresolved-label error (srd038 R1.5).
	out, ok := view.Lookup("no_such_step")
	require.False(t, ok)
	require.Equal(t, "", out)
}

func TestCommandStateViewDuplicateLabelMostRecentWins(t *testing.T) {
	t.Parallel()
	view := NewCommandStateView(cmdStateExecution())

	// Two different commands share an authored label; the later step (vec-2)
	// wins by step order, proving lookup uses Entry.Label rather than command.
	out, ok := view.Lookup("query_embedding")
	require.True(t, ok)
	require.Equal(t, "vec-2", out)
}

func TestCommandStateViewCommandNameFallbackAndAddressCollision(t *testing.T) {
	t.Parallel()

	view := NewCommandStateView(Execution{
		{CommandName: "collect", Label: "shared", Result: commandStateDigest("older-label")},
		{CommandName: "shared", Label: "publish", Result: commandStateDigest("newer-command")},
	})

	out, ok := view.Lookup("collect")
	require.True(t, ok, "labeled entries remain addressable by command name")
	require.Equal(t, "older-label", out)

	out, ok = view.Lookup("shared")
	require.True(t, ok)
	require.Equal(t, "newer-command", out, "one newest-to-oldest scan governs address collisions")

	unlabeled := NewCommandStateView(Execution{{
		CommandName: "unlabeled_step",
		Result:      commandStateDigest("unlabeled"),
	}})
	out, ok = unlabeled.Lookup("unlabeled_step")
	require.True(t, ok)
	require.Equal(t, "unlabeled", out)
}

func TestCommandStateViewEmptyLog(t *testing.T) {
	t.Parallel()
	view := NewCommandStateView(nil)

	out, ok := view.Lookup("embed_query")
	require.False(t, ok)
	require.Equal(t, "", out)
}

// TestCommandStateViewRehydratesAcrossInMemoryCheckpoint proves the view built
// from an execution restored by the checkpoint port resolves identical labels to
// the view built from the live log, so a resumed run reads the same command
// state (srd038 R1.4, backed by srd035/srd036 Load).
func TestCommandStateViewRehydratesAcrossInMemoryCheckpoint(t *testing.T) {
	t.Parallel()
	exec := cmdStateExecution()
	live := NewCommandStateView(exec)

	cp := &InMemoryCheckpoint{}
	require.NoError(t, cp.Save(Position{}, exec))
	_, restored, err := cp.Load()
	require.NoError(t, err)
	rehydrated := NewCommandStateView(restored)

	for _, label := range []string{"query_embedding", "record_event", "missing"} {
		liveOut, liveOK := live.Lookup(label)
		rehOut, rehOK := rehydrated.Lookup(label)
		require.Equal(t, liveOK, rehOK, "label %q resolves the same after rehydration", label)
		require.Equal(t, liveOut, rehOut, "label %q output matches after rehydration", label)
	}
}

// TestInjectCommandStateOnlyForAwareCommands proves the engine injects the view
// exactly into commands that opt in through CommandStateAware.
func TestInjectCommandStateOnlyForAwareCommands(t *testing.T) {
	t.Parallel()
	aware := &commandStateAwareStub{}
	injectCommandState(aware, cmdStateExecution())
	require.NotNil(t, aware.view, "an aware command receives the view")

	out, ok := aware.view.Lookup("query_embedding")
	require.True(t, ok)
	require.Equal(t, "vec-2", out)

	// A plain command that does not implement CommandStateAware is untouched.
	plain := &commandStatePlainStub{}
	require.NotPanics(t, func() { injectCommandState(plain, cmdStateExecution()) })
}

type commandStateAwareStub struct {
	view CommandStateView
}

func (c *commandStateAwareStub) Name() string                       { return "aware" }
func (c *commandStateAwareStub) Execute() Result                    { return Result{} }
func (c *commandStateAwareStub) Undo(prior Result) Result           { return NoopUndo(c.Name()) }
func (c *commandStateAwareStub) SetCommandState(v CommandStateView) { c.view = v }

var _ CommandStateAware = (*commandStateAwareStub)(nil)

type commandStatePlainStub struct{}

func (c *commandStatePlainStub) Name() string             { return "plain" }
func (c *commandStatePlainStub) Execute() Result          { return Result{} }
func (c *commandStatePlainStub) Undo(prior Result) Result { return NoopUndo(c.Name()) }

func TestResolveFromSelectorTypedErrors(t *testing.T) {
	t.Parallel()

	t.Run("unresolved label", func(t *testing.T) {
		t.Parallel()
		_, err := ResolveFromSelector(NewCommandStateView(nil), "$from(missing).mapped.id")
		var target *UnresolvedLabelError
		require.ErrorAs(t, err, &target)
		require.Equal(t, "missing", target.Label)
		require.ErrorContains(t, err, `$from(missing).mapped.id`)
	})

	t.Run("unresolved path", func(t *testing.T) {
		t.Parallel()
		view := NewCommandStateView(Execution{{
			CommandName: "load",
			Label:       "source",
			Result:      commandStateDigest(`{"mapped":{}}`),
		}})
		_, err := ResolveFromSelector(view, "$from(source).mapped.id")
		var target *UnresolvedPathError
		require.True(t, errors.As(err, &target))
		require.Equal(t, "source", target.Label)
		require.Equal(t, "mapped.id", target.Path)
		require.ErrorContains(t, err, `$from(source).mapped.id`)
	})
}
