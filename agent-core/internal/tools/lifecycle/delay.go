// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	defaultDelayElapsedSignal = core.Signal("DelayElapsed")
	defaultDeadlineSignal     = core.Signal("DeadlineReached")
)

// DelayConfig declares one bounded, cancellation-aware timer boundary. When a
// deadline source is present, each dispatch still waits at most Duration and
// reports when the authored window from that source has expired.
type DelayConfig struct {
	Duration       string `json:"duration"`
	Deadline       string `json:"deadline,omitempty"`
	DeadlineSource string `json:"deadline_source,omitempty"`
	ElapsedSignal  string `json:"elapsed_signal,omitempty"`
	DeadlineSignal string `json:"deadline_signal,omitempty"`
}

type delayBuilder struct {
	toolName string
	config   DelayConfig
}

func newDelayBuilder(toolName string, config DelayConfig) (core.Builder, error) {
	duration, err := positiveDuration(config.Duration)
	if err != nil {
		return nil, fmt.Errorf("tool %q (delay) duration: %w", toolName, err)
	}
	if config.Deadline != "" {
		if _, err := positiveDuration(config.Deadline); err != nil {
			return nil, fmt.Errorf("tool %q (delay) deadline: %w", toolName, err)
		}
		if _, _, ok := core.ParseFromSelector(config.DeadlineSource); !ok {
			return nil, fmt.Errorf("tool %q (delay) deadline_source must be a $from(label).path selector", toolName)
		}
	} else if config.DeadlineSource != "" {
		return nil, fmt.Errorf("tool %q (delay) deadline_source requires deadline", toolName)
	}
	_ = duration
	return delayBuilder{toolName: toolName, config: config}, nil
}

func (b delayBuilder) Build(_ core.Result) core.Command {
	return &delayCommand{toolName: b.toolName, config: b.config}
}

type delayCommand struct {
	toolName     string
	config       DelayConfig
	commandState core.CommandStateView
}

func (c *delayCommand) Name() string { return c.toolName }

func (c *delayCommand) SetCommandState(view core.CommandStateView) { c.commandState = view }

func (c *delayCommand) Undo(_ core.Result) core.Result { return core.NoopUndo(c.toolName) }

func (c *delayCommand) Execute() core.Result { return c.ExecuteContext(context.Background()) }

func (c *delayCommand) ExecuteContext(ctx context.Context) core.Result {
	duration, _ := positiveDuration(c.config.Duration)
	wait := duration
	deadlineSignal := configuredSignal(c.config.DeadlineSignal, defaultDeadlineSignal)

	if c.config.Deadline != "" {
		remaining, err := c.remaining()
		if err != nil {
			return delayError(c.toolName, err)
		}
		if remaining <= 0 {
			return delayResult(c.toolName, deadlineSignal, 0, true)
		}
		if remaining < wait {
			wait = remaining
		}
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return delayError(c.toolName, ctx.Err())
	case <-timer.C:
	}

	if c.config.Deadline != "" && wait < duration {
		return delayResult(c.toolName, deadlineSignal, wait, true)
	}
	return delayResult(
		c.toolName,
		configuredSignal(c.config.ElapsedSignal, defaultDelayElapsedSignal),
		wait,
		false,
	)
}

func (c *delayCommand) remaining() (time.Duration, error) {
	value, err := core.ResolveFromSelector(c.commandState, c.config.DeadlineSource)
	if err != nil {
		return 0, err
	}
	startedAt, ok := value.(string)
	if !ok {
		return 0, fmt.Errorf("delay deadline source resolved to %T, want string", value)
	}
	start, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return 0, fmt.Errorf("delay deadline source is not RFC3339: %w", err)
	}
	window, _ := positiveDuration(c.config.Deadline)
	return time.Until(start.Add(window)), nil
}

func positiveDuration(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return duration, nil
}

func configuredSignal(value string, fallback core.Signal) core.Signal {
	if value == "" {
		return fallback
	}
	return core.Signal(value)
}

func delayResult(name string, signal core.Signal, waited time.Duration, deadline bool) core.Result {
	output, _ := json.Marshal(map[string]interface{}{
		"waited":           waited.String(),
		"deadline_reached": deadline,
	})
	return core.Result{Signal: signal, CommandName: name, Output: string(output)}
}

func delayError(name string, err error) core.Result {
	return core.Result{Signal: core.CommandError, CommandName: name, Output: err.Error(), Err: err}
}

var _ core.CommandStateAware = (*delayCommand)(nil)
