// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package checkpoint

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Config holds the agent binary's checkpoint and resume flags. RegisterFlags
// binds them onto a caller-supplied flag set; the package never touches a
// global flagset. This package does not import internal/tools (boundaries.yaml);
// Dolt word identity stays at the composition root.
type Config struct {
	DoltDSN          string // --dolt-dsn
	ResumeCheckpoint string // --resume-checkpoint
	ResumeSignal     string // --resume-signal
}

// RegisterFlags defines the checkpoint flags on fs. Callers must invoke this
// from cmd/agent; nothing registers at import time.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.DoltDSN, "dolt-dsn", "", "MySQL-wire DSN to a dolt sql-server for the persistent checkpoint backend (default: no persistence)")
	fs.StringVar(&c.ResumeCheckpoint, "resume-checkpoint", "", "checkpoint ID to resume from")
	fs.StringVar(&c.ResumeSignal, "resume-signal", "", "resume signal override (default: required machine resume_signal)")
}

// Closeable is a Checkpoint that owns a Close, used by the OpenDolt test seam.
type Closeable interface {
	core.Checkpoint
	Close() error
}

// Opened is a resolved Checkpoint port plus an optional closer and diagnostic label.
type Opened struct {
	core.Checkpoint
	CloseFunc func() error
	Label     string
}

// Close runs CloseFunc when present.
func (o Opened) Close() error {
	if o.CloseFunc == nil {
		return nil
	}
	return o.CloseFunc()
}

// OpenDolt opens the persistent Dolt backend. Tests replace this seam.
var OpenDolt = func(dsn, runID string, terminal func(core.State) bool) (Closeable, error) {
	return core.OpenDoltCheckpoint(dsn, runID, terminal)
}

// Open returns the typed Checkpoint port for the run: the Dolt-backed
// persistent backend when --dolt-dsn is configured, otherwise NoopCheckpoint
// so a run without persistence keeps disabled-mode behavior
// (srd035-checkpoint-port R5.1, srd036-dolt-state-persistence R1).
func (c Config) Open(machine core.MachineSpec, runID string) (Opened, error) {
	return c.OpenRun(runID, terminalPredicate(machine))
}

// OpenRun opens the configured backend for runID. A nil terminal is valid for
// a read/revert handle that must not merge the run branch.
func (c Config) OpenRun(runID string, terminal func(core.State) bool) (Opened, error) {
	if strings.TrimSpace(c.DoltDSN) == "" {
		return Opened{Checkpoint: core.NoopCheckpoint{}}, nil
	}
	cp, err := OpenDolt(c.DoltDSN, runID, terminal)
	if err != nil {
		return Opened{}, fmt.Errorf("open dolt checkpoint: %w", err)
	}
	return Opened{
		Checkpoint: cp,
		CloseFunc:  cp.Close,
		Label:      "loop checkpoint",
	}, nil
}

// ResumeID returns the --resume-checkpoint value when set. "latest" is rejected.
func (c Config) ResumeID() (string, error) {
	id := strings.TrimSpace(c.ResumeCheckpoint)
	if id == "" {
		return "", nil
	}
	if id == "latest" {
		return "", fmt.Errorf("--resume-checkpoint %q is unsupported; provide an explicit run id", id)
	}
	return id, nil
}

func terminalPredicate(machine core.MachineSpec) func(core.State) bool {
	terminal := make(map[core.State]bool, len(machine.TerminalStates))
	for _, s := range machine.TerminalStates {
		terminal[core.State(s)] = true
	}
	return func(s core.State) bool { return terminal[s] }
}
