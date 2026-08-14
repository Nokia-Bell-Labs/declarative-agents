// Copyright (c) 2026 Nokia. All rights reserved.

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

func TestLoopParallelForEachIsBoundedAndJoinsInInputOrder(t *testing.T) {
	t.Parallel()
	spec := parallelIteratorMachine(t, ForEachCollectAll)
	tracker := &parallelItemTracker{
		failOn: "beta",
		delays: map[string]time.Duration{
			"alpha": 40 * time.Millisecond, "beta": 5 * time.Millisecond, "gamma": 15 * time.Millisecond,
		},
	}
	checkpoint := &InMemoryCheckpoint{}
	trace := &loopRecorder{}
	result, err := Loop(LoopParams{
		MachineSpec: &spec,
		Registry:    parallelIteratorRegistry([]string{"alpha", "beta", "gamma"}, tracker),
		Trace:       trace,
		Budget:      Budget{MaxIterations: 20},
		Checkpoint:  checkpoint,
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, 2, tracker.maxActiveCount())
	require.Equal(t, []string{"beta", "gamma", "alpha"}, tracker.completedValues())
	_, execution, err := checkpoint.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"list", "item", "item", "item", "for_each.join"}, entryCommands(execution))
	require.Equal(t, []string{"list", "item", "item", "item", "for_each.join"}, eventCommands(result.Events))
	require.Equal(t, []string{"", "item", "item", "item", ""}, entryReceipts(execution))
	require.Equal(t, []string{"alpha", "beta", "gamma"}, entryOutputs(execution)[1:4])
	require.Equal(t, Signal("ItemsPartial"), execution[len(execution)-1].Result.Signal)
	var joined struct {
		Items []struct {
			Index int `json:"index"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(execution[len(execution)-1].Result.Output), &joined))
	require.Equal(t, []int{0, 1, 2}, []int{joined.Items[0].Index, joined.Items[1].Index, joined.Items[2].Index})
	require.Len(t, trace.spans, 6, "the run and every list, item, and join dispatch receive spans")
}

func TestLoopParallelForEachFailFastCancelsAndJoinsChildren(t *testing.T) {
	t.Parallel()
	spec := parallelIteratorMachine(t, ForEachFailFast)
	tracker := &parallelItemTracker{
		failOn: "alpha",
		delays: map[string]time.Duration{
			"alpha": 5 * time.Millisecond, "beta": time.Second, "gamma": time.Second,
		},
	}
	start := time.Now()
	result, err := Loop(LoopParams{
		MachineSpec: &spec,
		Registry:    parallelIteratorRegistry([]string{"alpha", "beta", "gamma"}, tracker),
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 20},
		Checkpoint:  &InMemoryCheckpoint{},
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusFailed, result.Status)
	require.Less(t, time.Since(start), 500*time.Millisecond)
	require.Equal(t, 0, tracker.activeCount(), "loop returned before every started child joined")
	require.Contains(t, tracker.startedValues(), "alpha")
	require.GreaterOrEqual(t, len(tracker.startedValues()), 2)
}

func TestLoopParallelForEachRejectsSerialDispatchBeforeExecution(t *testing.T) {
	t.Parallel()
	spec := parallelIteratorMachine(t, ForEachFailFast)
	sharedConversation := []string{"existing"}
	command := &serialIteratorCommand{conversation: &sharedConversation}
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "list"}, iteratorListBuilder{items: []string{"alpha", "beta"}})
	registry.Register(ToolSpec{Name: "item"}, serialIteratorBuilder{command: command})
	checkpoint := &InMemoryCheckpoint{}

	result, err := Loop(LoopParams{
		MachineSpec: &spec, Registry: registry, Trace: &loopRecorder{},
		Budget: Budget{MaxIterations: 20}, Checkpoint: checkpoint,
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusFailed, result.Status)
	require.Zero(t, command.executions)
	require.Equal(t, []string{"existing"}, sharedConversation)
	_, execution, err := checkpoint.Load()
	require.NoError(t, err)
	var joined struct {
		Items []struct {
			Result struct {
				Error string `json:"error"`
			} `json:"result"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(execution[len(execution)-1].Result.Output), &joined))
	require.Equal(t, `iterator action "item" requires serial dispatch and cannot run in parallel mode`,
		joined.Items[0].Result.Error)
}

func TestLoopSequentialForEachAllowsSerialDispatch(t *testing.T) {
	t.Parallel()
	spec, err := ParseMachineSpec([]byte(iteratorMachineYAML))
	require.NoError(t, err)
	sharedConversation := []string{"existing"}
	command := &serialIteratorCommand{conversation: &sharedConversation}
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "list"}, iteratorListBuilder{items: []string{"alpha", "beta"}})
	registry.Register(ToolSpec{Name: "item"}, serialIteratorBuilder{command: command})

	result, err := Loop(LoopParams{
		MachineSpec: &spec, Registry: registry, Trace: &loopRecorder{},
		Budget: Budget{MaxIterations: 20},
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, 2, command.executions)
	require.Equal(t, []string{"existing", "turn", "turn"}, sharedConversation)
}

type serialIteratorBuilder struct {
	command *serialIteratorCommand
}

func (b serialIteratorBuilder) Build(Result) Command { return b.command }

type serialIteratorCommand struct {
	conversation *[]string
	executions   int
}

func (c *serialIteratorCommand) Name() string        { return "item" }
func (c *serialIteratorCommand) SerialDispatchOnly() {}
func (c *serialIteratorCommand) Undo(Result) Result  { return NoopUndo(c.Name()) }
func (c *serialIteratorCommand) Execute() Result {
	c.executions++
	*c.conversation = append(*c.conversation, "turn")
	return Result{Signal: "ItemDone", CommandName: c.Name()}
}

func parallelIteratorMachine(t *testing.T, failure string) MachineSpec {
	t.Helper()
	specYAML := strings.Replace(
		iteratorMachineYAML,
		"mode: sequential",
		"mode: parallel\n      max_concurrency: 2",
		1,
	)
	if failure == ForEachCollectAll {
		specYAML = strings.Replace(specYAML, "failure: fail_fast", "failure: collect_all", 1)
		specYAML = strings.Replace(specYAML,
			"signals: [Seed, ItemsReady, ItemDone, CommandError, ItemsDone, ItemsEmpty]",
			"signals: [Seed, ItemsReady, ItemDone, ItemFailed, CommandError, ItemsDone, ItemsPartial, ItemsEmpty]", 1)
		specYAML = strings.Replace(specYAML, "failed: CommandError\n          empty:",
			"partial: ItemsPartial\n          failed: CommandError\n          empty:", 1)
		specYAML += "\n  - state: Joined\n    signal: ItemsPartial\n    next: Done\n"
	}
	spec, err := ParseMachineSpec([]byte(specYAML))
	require.NoError(t, err)
	return spec
}

func parallelIteratorRegistry(items []string, tracker *parallelItemTracker) *Registry {
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "list"}, iteratorListBuilder{items: items})
	registry.Register(ToolSpec{Name: "item"}, parallelItemBuilder{tracker: tracker})
	return registry
}

type parallelItemBuilder struct {
	tracker *parallelItemTracker
}

func (b parallelItemBuilder) Build(Result) Command {
	return &parallelItemCommand{tracker: b.tracker}
}

type parallelItemCommand struct {
	tracker *parallelItemTracker
	view    CommandStateView
}

func (c *parallelItemCommand) Name() string                          { return "item" }
func (c *parallelItemCommand) SetCommandState(view CommandStateView) { c.view = view }
func (c *parallelItemCommand) Undo(Result) Result                    { return NoopUndo(c.Name()) }
func (c *parallelItemCommand) Execute() Result {
	return c.ExecuteContext(context.Background())
}
func (c *parallelItemCommand) ExecuteContext(ctx context.Context) Result {
	value, err := ResolveFromSelector(c.view, "$from(item).name")
	if err != nil {
		return Result{Signal: CommandError, Err: err}
	}
	name, _ := value.(string)
	c.tracker.start(name)
	defer c.tracker.finish(name)
	select {
	case <-time.After(c.tracker.delay(name)):
	case <-ctx.Done():
		return Result{Signal: CommandError, Err: ctx.Err(), Output: name}
	}
	if name == c.tracker.failOn {
		return Result{Signal: "ItemFailed", Output: name, Receipt: "item"}
	}
	return Result{Signal: "ItemDone", Output: name, Receipt: "item"}
}

type parallelItemTracker struct {
	mu        sync.Mutex
	delays    map[string]time.Duration
	failOn    string
	active    int
	maxActive int
	started   []string
	completed []string
}

func (t *parallelItemTracker) start(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active++
	t.started = append(t.started, name)
	if t.active > t.maxActive {
		t.maxActive = t.active
	}
}

func (t *parallelItemTracker) finish(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active--
	t.completed = append(t.completed, name)
}

func (t *parallelItemTracker) delay(name string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.delays[name]
}

func (t *parallelItemTracker) activeCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

func (t *parallelItemTracker) maxActiveCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.maxActive
}

func (t *parallelItemTracker) startedValues() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.started...)
}

func (t *parallelItemTracker) completedValues() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.completed...)
}

func entryOutputs(execution Execution) []string {
	values := make([]string, len(execution))
	for i := range execution {
		values[i] = execution[i].Result.Output
	}
	return values
}
