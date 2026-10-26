// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/inprocess"
)

// inProcessMu serializes in-process runs: a runner shares this process's
// working directory and globals, which a subprocess would have had alone.
var inProcessMu sync.Mutex

// run runs spec in this process when a build registered a runner for its
// binary, and as a subprocess otherwise (srd023 R8).
func (c *ExecCmd) run(ctx context.Context, spec subprocess.Spec) *subprocess.Result {
	if runner, ok := inprocess.Lookup(spec.Binary); ok {
		return runInProcess(ctx, runner, spec)
	}
	return subprocess.Run(ctx, spec)
}

// runInProcess runs spec through the runner registered for its binary and
// reports it as subprocess.Run would: combined output on Stdout and the
// exit code. A runner cannot take extra environment variables, which a
// subprocess would have had alone, so a spec that declares any is refused.
func runInProcess(ctx context.Context, runner inprocess.Runner, spec subprocess.Spec) *subprocess.Result {
	if len(spec.Env) > 0 {
		return &subprocess.Result{Err: fmt.Errorf("in-process binary %s takes no declared environment", spec.Binary)}
	}
	inProcessMu.Lock()
	defer inProcessMu.Unlock()
	var out bytes.Buffer
	start := time.Now()
	code := runner(ctx, inprocess.Call{Dir: spec.Dir, Args: spec.Args, Stdin: strings.NewReader(spec.Stdin), Out: &out})
	return &subprocess.Result{Stdout: out.String(), ExitCode: code, Duration: time.Since(start), Started: true}
}
