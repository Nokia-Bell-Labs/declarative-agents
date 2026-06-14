// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// RegisterToolDefs registers all tool definitions with the given registry.
// root is the working directory for all tools (overridden by ToolDef.Dir).
func RegisterToolDefs(reg *core.Registry, root string, defs []ToolDef) {
	for _, td := range defs {
		spec := td.ToToolSpec()
		builder := &ExecBuilder{Def: td, Root: root}
		reg.Register(spec, builder)
	}
}

// ExecBuilder is the generic Builder for YAML-defined tools.
type ExecBuilder struct {
	Def  ToolDef
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

// ExecCmd is the generic Command for YAML-defined tools.
type ExecCmd struct {
	def    ToolDef
	root   string
	params map[string]string
}

func (c *ExecCmd) Name() string { return c.def.Name }
func (c *ExecCmd) Undo() core.Result {
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
		err := fmt.Errorf("undo %s requires compensating action: %s", c.Name(), c.def.Undo.Description)
		return core.Result{
			Signal:      core.CommandError,
			CommandName: c.Name(),
			Output:      err.Error(),
			Err:         err,
		}
	default:
		err := fmt.Errorf("undo %s: unsupported undo strategy %q", c.Name(), c.def.Undo.Strategy)
		return core.Result{
			Signal:      core.CommandError,
			CommandName: c.Name(),
			Output:      err.Error(),
			Err:         err,
		}
	}
}

func (c *ExecCmd) UndoMemento() (core.UndoMemento, error) {
	switch c.def.Undo.Strategy {
	case "", "noop":
		return core.NoopUndoMemento(c.Name()), nil
	case "workspace_restore":
		return core.NewUndoMemento(c.Name(), core.UndoMementoReversible, workspaceRestorePayload(c.def))
	case "compensating_action":
		payload := any(workspaceRestorePayload(c.def))
		if c.def.Undo.Payload == "boundary_compensation" {
			payload = execBoundaryCompensationPayload(c.def, c.params)
		}
		memento, err := core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, payload)
		if err != nil {
			return core.UndoMemento{}, err
		}
		memento.Description = c.def.Undo.Description
		return memento, nil
	default:
		return core.UndoMemento{}, fmt.Errorf("%w: unsupported undo strategy %q for %s", core.ErrUndoMementoIncompatible, c.def.Undo.Strategy, c.Name())
	}
}

func execBoundaryCompensationPayload(def ToolDef, params map[string]string) BoundaryCompensationPayload {
	workspacePayload := workspaceRestorePayload(def)
	compensation := BoundaryCompensation{
		Strategy:       def.Undo.Strategy,
		Reason:         def.Undo.Description,
		Requires:       append([]string(nil), def.Undo.Requires...),
		WorkspacePaths: append([]string(nil), workspacePayload.WorkspaceRestore.Paths...),
		IssueID:        params["id"],
	}
	return BoundaryCompensationPayload{BoundaryCompensation: compensation}
}

func workspaceRestorePayload(def ToolDef) workspaceUndoPayload {
	payload := workspaceUndoPayload{}
	for _, effect := range def.SideEffects.Items {
		payload.WorkspaceRestore.Paths = append(payload.WorkspaceRestore.Paths, effect.Paths...)
	}
	if len(payload.WorkspaceRestore.Paths) == 0 {
		payload.WorkspaceRestore.Paths = []string{"."}
	}
	return payload
}

func (c *ExecCmd) Execute() core.Result {
	dir := c.root
	if c.def.Dir != "" {
		if filepath.IsAbs(c.def.Dir) {
			dir = c.def.Dir
		} else {
			dir = filepath.Join(c.root, c.def.Dir)
		}
	}

	if err := c.checkPrecondition(dir); err != nil {
		return core.Result{
			Output:      err.Error(),
			Signal:      core.ToolFailed,
			CommandName: c.def.Name,
		}
	}

	args := c.buildArgs()

	cmd := exec.Command(c.def.Binary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	res := SubprocessResult(c.def.Name, out, err)
	if c.def.OutputCap > 0 {
		res.Output = CapOutput(res.Output, c.def.OutputCap)
	}
	return res
}

func (c *ExecCmd) buildArgs() []string {
	args := make([]string, len(c.def.Args))
	copy(args, c.def.Args)

	mappings := c.def.ExtractParamMappings()
	for _, pm := range mappings {
		val, ok := c.params[pm.Name]
		if !ok {
			continue
		}
		if pm.BoolFlag {
			args = append(args, pm.Flag)
		} else if pm.Positional {
			args = append(args, val)
		} else {
			args = append(args, pm.Flag, val)
		}
	}

	return args
}

func (c *ExecCmd) checkPrecondition(dir string) error {
	switch c.def.Precondition {
	case "git_repo":
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("not a git repository: %s", dir)
			}
			return fmt.Errorf("checking git repo %s: %v", dir, err)
		}
		return nil
	case "":
		return nil
	default:
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err != nil {
			return fmt.Errorf("precondition %q failed: %v", c.def.Precondition, err)
		}
		return nil
	}
}
