// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/inprocess"
)

func init() {
	inprocess.Register("inprocess-test-word", func(_ context.Context, call inprocess.Call) int {
		_, _ = fmt.Fprintf(call.Out, "dir=%s args=%s", call.Dir, strings.Join(call.Args, " "))
		if strings.Contains(strings.Join(call.Args, " "), "--refuse") {
			return 1
		}
		return 0
	})
}

func inProcessDef(args ...string) catalog.ToolDef {
	return catalog.ToolDef{
		Name:   "word",
		Binary: "inprocess-test-word",
		Args:   args,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"issue": map[string]interface{}{"type": "string", "flag": "--issue"},
			},
		},
	}
}

// An exec tool whose binary has a registered runner runs in process, with
// the args and directory a subprocess would have had, and is shaped as a
// subprocess's result: exit 0 is ToolDone with the output.
func TestExecRunsARegisteredBinaryInProcess(t *testing.T) {
	cmd := (&ExecBuilder{Def: inProcessDef("issue", "claim"), Root: "/work"}).Build(core.Result{Output: `{"parameters":{"issue":"17"}}`})
	res := cmd.Execute()
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t, "dir=/work args=issue claim --issue 17", res.Output)
}

// A non-zero exit code from the runner is ToolFailed, as a subprocess's is.
func TestExecInProcessRefusalIsToolFailed(t *testing.T) {
	res := (&ExecBuilder{Def: inProcessDef("issue", "claim", "--refuse"), Root: "/work"}).Build(core.Result{Output: `{"parameters":{}}`}).Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "--refuse")
}
