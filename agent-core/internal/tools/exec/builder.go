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

// ExecCmd is the generic Command for YAML-defined exec tools.
type ExecCmd struct {
	def    catalog.ToolDef
	root   string
	params map[string]string
	rec    monitor.ToolMetricsRecorder
}

func (c *ExecCmd) Name() string { return c.def.Name }

func (c *ExecCmd) Undo(_ core.Result) core.Result {
	switch c.def.Undo.Strategy {
	case "", "noop":
		return core.NoopUndo(c.Name())
	case "workspace_restore":
		return core.Result{
			Signal:      core.ToolDone,
			CommandName: c.Name(),
			Output:      "undo: workspace restore is handled by the rollback workspace layer",
		}
	case "compensating_action":
		return compensationUndo(c.Name(), c.def.Undo.Description)
	default:
		err := fmt.Errorf("undo %s: unsupported undo strategy %q", c.Name(), c.def.Undo.Strategy)
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
