// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"context"
	osexec "os/exec"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
)

// RunResult is the outcome of a process-group-managed subprocess. The transport
// itself lives in internal/support/subprocess; exec keeps this alias so its
// callers and the documented word-library seam (srd013) reference one type
// backed by one implementation (GH-1393).
type RunResult = subprocess.Result

// ProcGroupCmd configures cmd to run in its own process group for clean
// cancellation, delegating to the single implementation (srd013 R4.2).
func ProcGroupCmd(cmd *osexec.Cmd) { subprocess.SetProcGroup(cmd) }

// RunProcGroup runs a process-group-managed subprocess within timeout by way of
// the shared transport.
func RunProcGroup(ctx context.Context, timeout time.Duration, dir, name string, args ...string) *RunResult {
	return subprocess.Run(ctx, subprocess.Spec{Binary: name, Args: args, Dir: dir, Timeout: timeout})
}
