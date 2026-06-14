// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
)

// ServeUIBuilder creates serveUICmd instances. It is the bench
// equivalent of InvokeLLMBuilder — the command blocks waiting for
// human input through the web UI.
type ServeUIBuilder struct {
	BS *BenchState
}

func (b *ServeUIBuilder) Build(_ core.Result) core.Command {
	if b.BS == nil {
		return &failCmd{err: fmt.Errorf("serve_ui: BenchState not initialized")}
	}
	return &serveUICmd{bs: b.BS}
}

// serveUICmd starts the web server (if needed) and blocks on the
// action channel until the user submits an action through the UI.
type serveUICmd struct {
	bs *BenchState
}

func (c *serveUICmd) Name() string { return "serve_ui" }
func (c *serveUICmd) Undo() core.Result {
	err := fmt.Errorf("undo serve_ui requires server shutdown or compensation for the submitted user action")
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
}
func (c *serveUICmd) UndoMemento() (core.UndoMemento, error) {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy:   "server_shutdown_or_user_action_compensation",
		Reason:     "serve_ui waits on a live HTTP server and human action",
		Requires:   []string{"server_addr", "last_user_action"},
		ServerAddr: c.bs.Addr,
		UserAction: c.bs.LastAction.Type,
	}}
	memento, err := core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, payload)
	if err != nil {
		return core.UndoMemento{}, err
	}
	memento.Description = "stop the UI server or compensate work triggered by the submitted action"
	return memento, nil
}

func (c *serveUICmd) Execute() core.Result {
	c.bs.EnsureRunning()

	action := <-c.bs.ActionCh

	c.bs.LastAction = action

	return core.Result{
		Signal:      action.Signal(),
		Output:      action.String(),
		CommandName: "serve_ui",
	}
}

// ServeUIFactory returns a registry.BuiltinFactory that creates ServeUIBuilder
// instances. The factory extracts config values from the tool
// declaration YAML to configure the server.
func ServeUIFactory(bs *BenchState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg catalog.ServeUIToolConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if bs.Addr == "" && cfg.Addr != "" {
			bs.Addr = cfg.Addr
		}
		if bs.Srv.dataDir == "" && cfg.DataDir != "" {
			bs.Srv.dataDir = cfg.DataDir
		}
		if bs.Srv.configsDir == "" && cfg.ConfigsDir != "" {
			bs.Srv.configsDir = cfg.ConfigsDir
		}
		if bs.Srv.docsDir == "" && cfg.DocsDir != "" {
			bs.Srv.docsDir = cfg.DocsDir
		}
		if bs.Srv.profilesDir == "" && cfg.ProfilesDir != "" {
			bs.Srv.profilesDir = cfg.ProfilesDir
		}
		if bs.Srv.sourceDir == "" && cfg.SourceDir != "" {
			bs.Srv.sourceDir = cfg.SourceDir
		}
		return &ServeUIBuilder{BS: bs}, nil
	}
}

type failCmd struct {
	err error
}

func (f *failCmd) Name() string      { return "fail" }
func (f *failCmd) Undo() core.Result { return core.NoopUndo(f.Name()) }

func (f *failCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.CommandError,
		Err:         f.err,
		Output:      f.err.Error(),
		CommandName: "fail",
	}
}
