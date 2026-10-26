// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package inprocess lets a build of the agent serve an exec tool's binary
// from code linked into it. An exec declaration stays as it is, with its
// binary, args, and parameter flags; when a runner is registered under the
// binary's name, the exec tool calls the runner in this process instead of
// starting one, and shapes its output and exit code exactly as it shapes a
// subprocess's. A build that registers nothing runs every exec tool as a
// subprocess, as before.
package inprocess

import (
	"context"
	"io"
	"sync"
)

// Call is one command line for a runner: the args the exec tool built, the
// working directory the subprocess would have had, and its stdin.
type Call struct {
	Dir   string
	Args  []string
	Stdin io.Reader
	// Out receives the combined output a subprocess would have written to
	// stdout and stderr.
	Out io.Writer
}

// Runner runs one Call and returns the exit code a process would have had.
type Runner func(ctx context.Context, call Call) int

var (
	mu      sync.RWMutex
	runners = map[string]Runner{}
)

// Register serves binary with r. It is meant for init functions of the
// packages a build links in; registering a binary twice panics.
func Register(binary string, r Runner) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := runners[binary]; ok {
		panic("inprocess: binary " + binary + " registered twice")
	}
	runners[binary] = r
}

// Lookup returns the runner registered for binary.
func Lookup(binary string) (Runner, bool) {
	mu.RLock()
	defer mu.RUnlock()
	r, ok := runners[binary]
	return r, ok
}
