// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// RegisterToolDefs registers exec tool definitions with the given registry.
func RegisterToolDefs(reg *core.Registry, root string, defs []catalog.ToolDef) {
	for _, td := range defs {
		reg.Register(td.ToToolSpec(), &ExecBuilder{Def: td, Root: root})
	}
}

// ExecBuilder is the generic Builder for YAML-defined exec tools.
type ExecBuilder struct {
	Def  catalog.ToolDef
	Root string
}

// Build extracts parameters from the previous result and creates an ExecCmd.
func (b *ExecBuilder) Build(res core.Result) core.Command {
	mappings := b.Def.ExtractParamMappings()
	params := make(map[string]string)
	for _, pm := range mappings {
		val := ExtractStringParam(res.Output, pm.Name)
		if val == "" && pm.Default != "" {
			val = pm.Default
		}
		if val == "" && pm.Required {
			return &FailedParamCmd{ToolName: b.Def.Name, Missing: pm.Name}
		}
		if val != "" {
			params[pm.Name] = val
		}
	}
	return &ExecCmd{def: b.Def, root: b.Root, params: params}
}

// BuildReverser returns an exec command configured only for receipt-driven Undo:
// the receipt carries the undo strategy/description, so the rollback receipt
// walk needs no extracted params (core.Reverser; srd035-checkpoint-port R3).
func (b *ExecBuilder) BuildReverser() core.Command {
	return &ExecCmd{def: b.Def, root: b.Root}
}

// ExecCmd is the generic Command for YAML-defined exec tools.
type ExecCmd struct {
	def    catalog.ToolDef
	root   string
	params map[string]string
	rec    monitor.ToolMetricsRecorder
}

func (c *ExecCmd) Name() string { return c.def.Name }

// Undo reverses the exec effect using the tool-owned receipt on the prior
// Result, falling back to the declared undo contract for the live in-process
// path. It is best-effort per the declared reversibility tier; git-style DB
// state is reverted separately by DoltCheckpoint (srd036).
func (c *ExecCmd) Undo(prior core.Result) core.Result {
	strategy := c.def.Undo.Strategy
	description := c.def.Undo.Description
	if r, ok, err := decodeExecReceipt(prior.Receipt); err != nil {
		e := fmt.Errorf("undo %s: decode receipt: %w", c.Name(), err)
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: e.Error(), Err: e}
	} else if ok {
		strategy = r.Strategy
		description = r.Description
	}
	switch strategy {
	case "", "noop":
		return core.NoopUndo(c.Name())
	case "workspace_restore":
		return core.Result{
			Signal:      core.ToolDone,
			CommandName: c.Name(),
			Output:      "undo: workspace restore is handled by the Dolt revert of DB state",
		}
	case "compensating_action":
		return compensationUndo(c.Name(), description)
	default:
		err := fmt.Errorf("undo %s: unsupported undo strategy %q", c.Name(), strategy)
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
	}
}

func (c *ExecCmd) Execute() core.Result {
	dir := c.execDir()
	if err := c.checkPrecondition(dir); err != nil {
		return core.Result{Output: err.Error(), Signal: core.ToolFailed, CommandName: c.def.Name}
	}
	cmd := osexec.Command(c.def.Binary, c.buildArgs()...)
	cmd.Dir = dir
	start := time.Now()
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)
	res := SubprocessResult(c.def.Name, out, err)
	c.recordExecMetrics(duration, out, err)
	if c.def.OutputCap > 0 {
		res.Output = CapOutput(res.Output, c.def.OutputCap)
	}
	if res.Signal != core.CommandError {
		res.Receipt = c.encodeReceipt()
	}
	return res
}

func (c *ExecCmd) execDir() string {
	if c.def.Dir == "" {
		return c.root
	}
	if filepath.IsAbs(c.def.Dir) {
		return c.def.Dir
	}
	return filepath.Join(c.root, c.def.Dir)
}

func (c *ExecCmd) buildArgs() []string {
	args := append([]string(nil), c.def.Args...)
	for _, pm := range c.def.ExtractParamMappings() {
		val, ok := c.params[pm.Name]
		if !ok {
			continue
		}
		args = appendMappedArg(args, pm, val)
	}
	return args
}

func appendMappedArg(args []string, pm catalog.ParamMapping, val string) []string {
	switch {
	case pm.BoolFlag:
		return append(args, pm.Flag)
	case pm.Positional:
		return append(args, val)
	default:
		return append(args, pm.Flag, val)
	}
}

func (c *ExecCmd) checkPrecondition(dir string) error {
	switch c.def.Precondition {
	case "git_repo":
		return checkGitRepo(dir)
	case "":
		return nil
	default:
		if err := checkGitRepo(dir); err != nil {
			return fmt.Errorf("precondition %q failed: %v", c.def.Precondition, err)
		}
		return nil
	}
}

func checkGitRepo(dir string) error {
	gitPath := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not a git repository: %s", dir)
		}
		return fmt.Errorf("checking git repo %s: %v", dir, err)
	}
	return nil
}
