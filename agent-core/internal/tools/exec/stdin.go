// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	osexec "os/exec"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// DefaultStdinMaxBytes bounds command-state input when a declaration does not
// choose a smaller positive limit.
const DefaultStdinMaxBytes = 1 << 20

func (c *ExecCmd) resolveStdin() (string, error) {
	if c.def.StdinSource == "" {
		return "", nil
	}
	value, err := core.ResolveFromSelector(c.view, c.def.StdinSource)
	if err != nil {
		return "", fmt.Errorf("stdin_source %q: %w", c.def.StdinSource, err)
	}
	input, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("stdin_source %q resolved to %T, want string", c.def.StdinSource, value)
	}
	if input == "" {
		return "", fmt.Errorf("stdin_source %q resolved to an empty string", c.def.StdinSource)
	}
	limit := c.def.StdinMaxBytes
	if limit == 0 {
		limit = DefaultStdinMaxBytes
	}
	if len(input) > limit {
		return "", fmt.Errorf("stdin_source %q is %d bytes, exceeds limit %d", c.def.StdinSource, len(input), limit)
	}
	return input, nil
}

func runExecProcess(
	ctx context.Context,
	def catalog.ToolDef,
	dir string,
	args []string,
	stdin string,
) ([]byte, time.Duration, error) {
	cmd := osexec.CommandContext(ctx, def.Binary, args...)
	cmd.Dir = dir
	ProcGroupCmd(cmd)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	start := time.Now()
	if stdin == "" {
		err := cmd.Run()
		return output.Bytes(), time.Since(start), err
	}
	err := runWithStdin(cmd, stdin)
	return output.Bytes(), time.Since(start), err
}

func runWithStdin(cmd *osexec.Cmd, input string) error {
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = pipe.Close()
		return err
	}
	written := make(chan struct{})
	go func() {
		_, _ = io.WriteString(pipe, input)
		_ = pipe.Close()
		close(written)
	}()
	err = cmd.Wait()
	_ = pipe.Close()
	<-written
	return err
}

func shapeExecOutput(def catalog.ToolDef, res core.Result, runErr error) core.Result {
	if def.Output.Mode != "structured" {
		return res
	}
	encoded, err := json.Marshal(struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}{Output: res.Output, ExitCode: exitCode(runErr)})
	if err != nil {
		res.Err = fmt.Errorf("%s: encode structured output: %w", def.Name, err)
		res.Signal = core.CommandError
		return res
	}
	res.Output = string(encoded)
	return res
}
