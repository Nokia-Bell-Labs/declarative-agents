// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
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

// ServeUIFactory returns a BuiltinFactory that creates ServeUIBuilder
// instances. The factory extracts config values from the tool
// declaration YAML to configure the server.
func ServeUIFactory(bs *BenchState) stl.BuiltinFactory {
	return func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		// YAML config values are defaults — only apply when the field
		// wasn't already set by CLI flags via ServerConfig.
		if bs.Addr == "" {
			if addr, ok := def.Config["addr"].(string); ok && addr != "" {
				bs.Addr = addr
			}
		}
		if bs.Srv.dataDir == "" {
			if dir, ok := def.Config["data_dir"].(string); ok && dir != "" {
				bs.Srv.dataDir = dir
			}
		}
		if bs.Srv.configsDir == "" {
			if dir, ok := def.Config["configs_dir"].(string); ok && dir != "" {
				bs.Srv.configsDir = dir
			}
		}
		if bs.Srv.docsDir == "" {
			if dir, ok := def.Config["docs_dir"].(string); ok && dir != "" {
				bs.Srv.docsDir = dir
			}
		}
		if bs.Srv.profilesDir == "" {
			if dir, ok := def.Config["profiles_dir"].(string); ok && dir != "" {
				bs.Srv.profilesDir = dir
			}
		}
		if bs.Srv.sourceDir == "" {
			if dir, ok := def.Config["source_dir"].(string); ok && dir != "" {
				bs.Srv.sourceDir = dir
			}
		}
		return &ServeUIBuilder{BS: bs}, nil
	}
}

type failCmd struct {
	err error
}

func (f *failCmd) Name() string { return "fail" }
func (f *failCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.CommandError,
		Err:         f.err,
		Output:      f.err.Error(),
		CommandName: "fail",
	}
}
