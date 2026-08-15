// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoopForEachDispatchesItemsInOrderAndJoins(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	order := &orderedItems{}
	params := iteratorLoopParams(t, cp, order)

	result, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, []string{"alpha", "beta"}, order.values())
	require.Equal(t, 4, result.Iterations, "list, two item dispatches, and join")
	_, execution, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"list", "item", "item", "for_each.join"}, entryCommands(execution))
	require.Equal(t, []string{"", "item", "item", ""}, entryReceipts(execution))
	require.Equal(t, []string{"items", "each_item", "each_item", "items_joined"}, entryLabels(execution))
}

func TestLoopForEachEmptyCollectionJoinsDirectlyToTerminal(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	params := iteratorLoopParams(t, cp, &orderedItems{})
	params.Registry = iteratorRegistry(nil, &orderedItems{})
	params.MachineSpec.Transitions[1].ForEach.Join.Next = "Done"

	result, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, State("Done"), result.FinalState)
	require.Equal(t, 2, result.Iterations, "list and empty join")
	require.Equal(t, []string{"list", "for_each.join"}, eventCommands(result.Events))
	_, execution, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"list", "for_each.join"}, entryCommands(execution))
	require.JSONEq(t, `{"failed":0,"items":[],"policy":"fail_fast","succeeded":0}`, execution[1].Result.Output)
}

func TestLoopForEachResumeContinuesAfterLastPersistedItem(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cp := &cancelAfterIteratorSave{cancel: cancel}
	order := &orderedItems{}
	params := iteratorLoopParams(t, cp, order)

	first, err := Loop(params, ctx)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, first.Status)
	require.Equal(t, []string{"alpha"}, order.values())

	params.InitialSignal = Approved
	resumed, err := LoadResume(params)
	require.NoError(t, err)
	require.NotNil(t, resumed.Params.InitialIterator)
	second, err := Loop(resumed.Params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, second.Status)
	require.Equal(t, []string{"alpha", "beta"}, order.values(), "completed item was not replayed")
}

func TestLoopForEachCollectAllJoinsPartialInInputOrder(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	order := &orderedItems{}
	specYAML := strings.Replace(iteratorMachineYAML, "failure: fail_fast", "failure: collect_all", 1)
	specYAML = strings.Replace(specYAML, "signals: [Seed, ItemsReady, ItemDone, CommandError, ItemsDone, ItemsEmpty]",
		"signals: [Seed, ItemsReady, ItemDone, ItemFailed, CommandError, ItemsDone, ItemsPartial, ItemsEmpty]", 1)
	specYAML = strings.Replace(specYAML, "failed: CommandError\n          empty:",
		"partial: ItemsPartial\n          failed: CommandError\n          empty:", 1)
	specYAML += "\n  - state: Joined\n    signal: ItemsPartial\n    next: Done\n"
	spec, err := ParseMachineSpec([]byte(specYAML))
	require.NoError(t, err)
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "list"}, iteratorListBuilder{items: []string{"alpha", "beta", "gamma"}})
	registry.Register(ToolSpec{Name: "item"}, iteratorItemBuilder{order: order, failOn: "beta"})

	result, err := Loop(LoopParams{
		MachineSpec: &spec, Registry: registry, Trace: &loopRecorder{},
		Budget: Budget{MaxIterations: 20}, Checkpoint: cp,
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, []string{"alpha", "beta", "gamma"}, order.values())
	_, execution, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, Signal("ItemsPartial"), execution[len(execution)-1].Result.Signal)
}

func TestLoopForEachItemHonorsCommandTimeout(t *testing.T) {
	t.Parallel()
	spec, err := ParseMachineSpec([]byte(iteratorMachineYAML))
	require.NoError(t, err)
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "list"},
		iteratorListBuilder{items: []string{"alpha"}})
	registry.Register(ToolSpec{Name: "item"}, activeCommandBuilder{
		command: &dispatchContextBlockingCmd{
			started: make(chan struct{}), finished: make(chan struct{}),
		},
	})
	result, err := Loop(LoopParams{
		MachineSpec: &spec, Registry: registry, Trace: &loopRecorder{},
		Budget: Budget{MaxIterations: 10}, CommandTimeout: time.Millisecond,
	}, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusFailed, result.Status)
	require.ErrorContains(t, result.LastError, "timeout executing context_blocking")
}

func TestLoopForEachAbortSignalStopsCollectAll(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	order := &orderedItems{}
	specYAML := strings.Replace(iteratorMachineYAML, "failure: fail_fast", "failure: collect_all", 1)
	specYAML = strings.Replace(specYAML, "abort_on: [CommandError]", "abort_on: [ItemFailed]", 1)
	specYAML = strings.Replace(specYAML, "signals: [Seed, ItemsReady, ItemDone, CommandError, ItemsDone, ItemsEmpty]",
		"signals: [Seed, ItemsReady, ItemDone, ItemFailed, CommandError, ItemsDone, ItemsPartial, ItemsEmpty]", 1)
	specYAML = strings.Replace(specYAML, "failed: CommandError\n          empty:",
		"partial: ItemsPartial\n          failed: CommandError\n          empty:", 1)
	specYAML += "\n  - state: Joined\n    signal: ItemsPartial\n    next: Done\n"
	spec, err := ParseMachineSpec([]byte(specYAML))
	require.NoError(t, err)
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "list"}, iteratorListBuilder{items: []string{"alpha", "beta", "gamma"}})
	registry.Register(ToolSpec{Name: "item"}, iteratorItemBuilder{order: order, failOn: "beta"})

	result, err := Loop(LoopParams{
		MachineSpec: &spec, Registry: registry, Trace: &loopRecorder{},
		Budget: Budget{MaxIterations: 20}, Checkpoint: cp,
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusFailed, result.Status)
	require.Equal(t, []string{"alpha", "beta"}, order.values())
}

func iteratorLoopParams(t *testing.T, checkpoint Checkpoint, order *orderedItems) LoopParams {
	t.Helper()
	spec, err := ParseMachineSpec([]byte(iteratorMachineYAML))
	require.NoError(t, err)
	return LoopParams{
		MachineSpec: &spec, Registry: iteratorRegistry([]string{"alpha", "beta"}, order),
		Trace: &loopRecorder{}, Budget: Budget{MaxIterations: 20}, Checkpoint: checkpoint,
	}
}

func iteratorRegistry(items []string, order *orderedItems) *Registry {
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "list"}, iteratorListBuilder{items: items})
	registry.Register(ToolSpec{Name: "item"}, iteratorItemBuilder{order: order})
	return registry
}

type iteratorListBuilder struct{ items []string }

func (b iteratorListBuilder) Build(Result) Command {
	return iteratorListCommand(b)
}

type iteratorListCommand struct{ items []string }

func (c iteratorListCommand) Name() string       { return "list" }
func (c iteratorListCommand) Undo(Result) Result { return NoopUndo(c.Name()) }
func (c iteratorListCommand) Execute() Result {
	values := make([]map[string]string, 0, len(c.items))
	for _, item := range c.items {
		values = append(values, map[string]string{"name": item})
	}
	output, _ := json.Marshal(map[string]interface{}{"items": values})
	return Result{
		Signal: "ItemsReady", CommandName: c.Name(),
		Output: string(output),
	}
}

type iteratorItemBuilder struct {
	order  *orderedItems
	failOn string
}

func (b iteratorItemBuilder) Build(Result) Command {
	return &iteratorItemCommand{order: b.order, failOn: b.failOn}
}

type iteratorItemCommand struct {
	order  *orderedItems
	view   CommandStateView
	failOn string
}

func (c *iteratorItemCommand) Name() string                          { return "item" }
func (c *iteratorItemCommand) SetCommandState(view CommandStateView) { c.view = view }
func (c *iteratorItemCommand) Undo(Result) Result                    { return NoopUndo(c.Name()) }
func (c *iteratorItemCommand) Execute() Result {
	value, err := ResolveFromSelector(c.view, "$from(item).name")
	if err != nil {
		return Result{Signal: CommandError, CommandName: c.Name(), Err: err}
	}
	name, _ := value.(string)
	c.order.add(name)
	if name == c.failOn {
		return Result{Signal: "ItemFailed", CommandName: c.Name(), Output: name}
	}
	return Result{Signal: "ItemDone", CommandName: c.Name(), Output: name, Receipt: "item"}
}

type orderedItems struct {
	mu    sync.Mutex
	items []string
}

func (o *orderedItems) add(item string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.items = append(o.items, item)
}

func (o *orderedItems) values() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.items...)
}

type cancelAfterIteratorSave struct {
	InMemoryCheckpoint
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelAfterIteratorSave) Save(position Position, execution Execution) error {
	err := c.InMemoryCheckpoint.Save(position, execution)
	if position.Snapshot.Iterator != nil && position.Snapshot.Iterator.NextIndex == 1 {
		c.once.Do(c.cancel)
	}
	return err
}

func entryCommands(execution Execution) []string {
	values := make([]string, len(execution))
	for i := range execution {
		values[i] = execution[i].CommandName
	}
	return values
}

func eventCommands(events []RunEvent) []string {
	values := make([]string, len(events))
	for i := range events {
		values[i] = events[i].CommandName
	}
	return values
}

func entryReceipts(execution Execution) []string {
	values := make([]string, len(execution))
	for i := range execution {
		values[i] = execution[i].Receipt
	}
	return values
}

func entryLabels(execution Execution) []string {
	values := make([]string, len(execution))
	for i := range execution {
		values[i] = execution[i].Label
	}
	return values
}

const iteratorMachineYAML = `
name: iterator-test
initial_state: Start
states: [Start, Loading, Iterating, Joined, {name: Done, run_status: succeeded}, {name: Failed, run_status: failed}]
terminal_states: [Done, Failed]
signals: [Seed, ItemsReady, ItemDone, CommandError, ItemsDone, ItemsEmpty]
transitions:
  - state: Start
    signal: Seed
    next: Loading
    action: list
    label: items
  - state: Loading
    signal: ItemsReady
    next: Iterating
    action: item
    label: each_item
    for_each:
      items: $from(items).items
      as: item
      mode: sequential
      failure: fail_fast
      continue_on: [ItemDone]
      abort_on: [CommandError]
      join:
        next: Joined
        label: items_joined
        signals:
          all_success: ItemsDone
          failed: CommandError
          empty: ItemsEmpty
  - state: Joined
    signal: ItemsDone
    next: Done
  - state: Joined
    signal: ItemsEmpty
    next: Done
  - state: Joined
    signal: CommandError
    next: Failed
`
