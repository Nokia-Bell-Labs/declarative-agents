// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// gitPreconditionTimeout bounds the git rev-parse precondition probe so a hung
// or slow git cannot stall dispatch.
const gitPreconditionTimeout = 5 * time.Second

// RegisterToolDefs registers exec tool definitions with the given registry.
func RegisterToolDefs(reg *core.Registry, root string, defs []catalog.ToolDef) {
	for _, td := range defs {
		reg.Register(td.ToToolSpec(), &ExecBuilder{Def: td, Root: root})
	}
}

// ExecBuilder is the generic Builder for YAML-defined exec tools.
type ExecBuilder struct {
	Def      catalog.ToolDef
	Root     string
	RootFunc func() string
}

// Build extracts adjacent parameters from the previous result and defers
// command-state sources until the command receives its view before dispatch.
func (b *ExecBuilder) Build(res core.Result) core.Command {
	mappings := b.Def.ExtractParamMappings()
	params := make(map[string]string)
	var sources []catalog.ParamMapping
	for _, pm := range mappings {
		if pm.Source != "" {
			sources = append(sources, pm)
			continue
		}
		val := ExtractStringParam(res.Output, pm.Name)
		if val == "" {
			if n := ExtractIntParam(res.Output, pm.Name); n != 0 {
				val = strconv.Itoa(n)
			}
		}
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
	return &ExecCmd{def: b.Def, root: b.Root, rootFunc: b.RootFunc, params: params, sources: sources}
}

// BuildReverser returns an exec command configured only for receipt-driven Undo:
// the receipt carries the undo strategy/description, so the rollback receipt
// walk needs no extracted params (core.Reverser; srd035-checkpoint-port R3).
func (b *ExecBuilder) BuildReverser() core.Command {
	return &ExecCmd{def: b.Def, root: b.Root, rootFunc: b.RootFunc}
}

// ExecCmd is the generic Command for YAML-defined exec tools.
type ExecCmd struct {
	def      catalog.ToolDef
	root     string
	rootFunc func() string
	params   map[string]string
	sources  []catalog.ParamMapping
	view     core.CommandStateView
	rec      monitor.ToolMetricsRecorder
}

func (c *ExecCmd) Name() string { return c.def.Name }

// SetCommandState supplies the view needed to resolve non-adjacent parameter
// sources after Build and before subprocess launch (srd023 R2.8, R3.9).
func (c *ExecCmd) SetCommandState(view core.CommandStateView) { c.view = view }

var (
	_ core.CommandStateAware = (*ExecCmd)(nil)
	_ core.ContextCommand    = (*ExecCmd)(nil)
)

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
	return c.ExecuteContext(context.Background())
}

// ExecuteContext resolves all command-state input before launch, then runs one
// binary whose process group is canceled and joined with the dispatch context.
func (c *ExecCmd) ExecuteContext(ctx context.Context) core.Result {
	stdin, env, err := c.resolveInputs()
	if err != nil {
		wrapped := fmt.Errorf("%s: resolve exec input: %w", c.Name(), err)
		return core.Result{
			Output:      wrapped.Error(),
			Signal:      core.CommandError,
			CommandName: c.Name(),
			Err:         wrapped,
		}
	}
	dir := c.execDir()
	if err := c.checkPrecondition(ctx, dir); err != nil {
		return core.Result{Output: err.Error(), Signal: core.ToolFailed, CommandName: c.def.Name}
	}
	run := subprocess.Run(ctx, subprocess.Spec{
		Binary:           c.def.Binary,
		Args:             c.buildArgs(),
		Dir:              dir,
		Env:              env,
		Stdin:            stdin,
		CombinedOutput:   true,
		NoDefaultTimeout: true,
	})
	res := SubprocessResult(c.def.Name, run)
	c.recordExecMetrics(run.Duration, len(run.Stdout), run.ExitCode)
	if c.def.OutputCap > 0 {
		res.Output = CapOutput(res.Output, c.def.OutputCap)
	}
	res = shapeExecOutput(c.def, res, run.ExitCode)
	if res.Signal != core.CommandError {
		res.Receipt = c.encodeReceipt(res.Output)
	}
	return res
}

func (c *ExecCmd) resolveInputs() (string, []string, error) {
	if err := c.resolveSourceParams(); err != nil {
		return "", nil, fmt.Errorf("parameter source: %w", err)
	}
	stdin, err := c.resolveStdin()
	if err != nil {
		return "", nil, err
	}
	env, err := c.resolveEnv()
	if err != nil {
		return "", nil, err
	}
	return stdin, env, nil
}

func (c *ExecCmd) resolveSourceParams() error {
	if c.params == nil {
		c.params = make(map[string]string)
	}
	for _, pm := range c.sources {
		delete(c.params, pm.Name)
		value, err := core.ResolveFromSelector(c.view, pm.Source)
		if err != nil {
			return fmt.Errorf("parameter %q from %q: %w", pm.Name, pm.Source, err)
		}
		resolved, ok := value.(string)
		if !ok {
			return fmt.Errorf(
				"parameter %q from %q resolved to %T, want string",
				pm.Name, pm.Source, value,
			)
		}
		if resolved == "" {
			resolved = pm.Default
		}
		if resolved == "" && pm.Required {
			return fmt.Errorf("required parameter %q from %q resolved to an empty string", pm.Name, pm.Source)
		}
		if resolved != "" {
			c.params[pm.Name] = resolved
		}
	}
	return nil
}

func (c *ExecCmd) execDir() string {
	root := c.root
	if c.rootFunc != nil {
		root = c.rootFunc()
	}
	if c.def.Dir == "" {
		return root
	}
	if filepath.IsAbs(c.def.Dir) {
		return c.def.Dir
	}
	return filepath.Join(root, c.def.Dir)
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

// checkPrecondition gates dispatch on the declared precondition. An unknown
// value is an error rather than a silent fall-through to the git check, so a
// typo surfaces at dispatch even if it reached here without load-time
// validation (GH-1381).
func (c *ExecCmd) checkPrecondition(ctx context.Context, dir string) error {
	switch c.def.Precondition {
	case "":
		return nil
	case "git_repo":
		return checkGitRepo(ctx, dir)
	default:
		return fmt.Errorf("unknown precondition %q", c.def.Precondition)
	}
}

// checkGitRepo asks git itself whether dir sits in a working tree, replacing an
// os.Stat on <dir>/.git. The stat rejected any subdirectory of a repository and
// accepted a worktree gitfile without resolving it; git rev-parse handles both,
// and reports the same failure git would (GH-1381).
func checkGitRepo(ctx context.Context, dir string) error {
	res := RunProcGroup(ctx, gitPreconditionTimeout, dir, "git", "rev-parse", "--git-dir")
	if res.Success() {
		return nil
	}
	if res.Err != nil {
		return fmt.Errorf("checking git repo %s: %w", dir, res.Err)
	}
	detail := strings.TrimSpace(res.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(res.Stdout)
	}
	if detail == "" {
		return fmt.Errorf("not a git repository: %s", dir)
	}
	return fmt.Errorf("not a git repository: %s: %s", dir, detail)
}
