// Copyright (c) 2026 Nokia. All rights reserved.

// Package core provides the generic agentic loop engine: a state
// machine, command dispatch, tool registry, budget tracking, and
// tracing. Domain-specific agents import core and supply their own
// states, signals, tools, and transition tables.
package core

import "time"

// State represents a position in the agentic loop lifecycle.
type State string

// Signal carries the outcome of a Command back to the state machine.
type Signal string

// Generic signals used by the loop engine itself.
const (
	Seed            Signal = "Seed"
	BudgetExhausted Signal = "BudgetExhausted"
	CommandError    Signal = "CommandError"
)

// Command is the single interface for all executable units of work.
type Command interface {
	Name() string
	Execute() Result
}

// Cost tracks resource consumption for a single Command dispatch.
type Cost struct {
	Duration  time.Duration `json:"duration"`
	TokensIn  int           `json:"tokens_in"`
	TokensOut int           `json:"tokens_out"`
	Dollars   float64       `json:"dollars"`
}

// Result carries the output of a Command execution.
type Result struct {
	Output      string
	Signal      Signal
	Cost        Cost
	Err         error
	CommandName string
}

// Builder constructs a ready-to-execute Command from the previous Result.
type Builder interface {
	Build(res Result) Command
}

// CommandResolver looks up a Builder by command name.
type CommandResolver interface {
	Resolve(name string) (Builder, bool)
}
