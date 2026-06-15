// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
)

const defaultAddr = ":18081"

// ToolConfig configures the serve_documentation boundary word.
type ToolConfig struct {
	Addr        string `json:"addr"`
	DocsDir     string `json:"docs_dir"`
	ConfigsDir  string `json:"configs_dir"`
	SourceDir   string `json:"source_dir"`
	ProfilePath string `json:"profile_path"`
}

// RegisterFactories registers Knowledge Manager documentation builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry) {
	br.Register("serve_documentation", ServeDocumentationFactory())
}

// ServeDocumentationFactory creates builders for the documentation UI host.
func ServeDocumentationFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		cfg, err := decodeToolConfig(def)
		if err != nil {
			return nil, err
		}
		return ServeDocumentationBuilder{Config: cfg}, nil
	}
}

// ServeDocumentationBuilder creates serve_documentation commands.
type ServeDocumentationBuilder struct {
	Config ToolConfig
}

func (b ServeDocumentationBuilder) Build(_ core.Result) core.Command {
	return serveDocumentationCmd{config: b.Config}
}

type serveDocumentationCmd struct {
	config ToolConfig
}

func (c serveDocumentationCmd) Name() string { return "serve_documentation" }

func (c serveDocumentationCmd) Undo() core.Result {
	return core.NoopUndo(c.Name())
}

func (c serveDocumentationCmd) Execute() core.Result {
	err := NewServer(HostConfig{
		Addr:       c.config.Addr,
		DocsDir:    c.config.DocsDir,
		ConfigsDir: c.config.ConfigsDir,
		SourceDir:  c.config.SourceDir,
		Workflow:   NewLazyProfileWorkflowRunner(c.config.ProfilePath),
	}).ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return core.Result{Signal: "ServerStopped", CommandName: c.Name()}
	}
	if err == nil {
		return core.Result{Signal: "ServerStopped", CommandName: c.Name()}
	}
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Err: err, Output: err.Error()}
}

func decodeToolConfig(def catalog.ToolDef) (ToolConfig, error) {
	var cfg ToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return ToolConfig{}, err
	}
	if cfg.Addr == "" {
		cfg.Addr = defaultAddr
	}
	if cfg.DocsDir == "" {
		return ToolConfig{}, fmt.Errorf("tool %q config requires docs_dir", def.Name)
	}
	if abs, err := filepath.Abs(cfg.DocsDir); err == nil {
		cfg.DocsDir = abs
	}
	if cfg.ConfigsDir == "" {
		cfg.ConfigsDir = "configs"
	}
	if abs, err := filepath.Abs(cfg.ConfigsDir); err == nil {
		cfg.ConfigsDir = abs
	}
	if cfg.SourceDir == "" {
		cfg.SourceDir = "."
	}
	if abs, err := filepath.Abs(cfg.SourceDir); err == nil {
		cfg.SourceDir = abs
	}
	if cfg.ProfilePath == "" {
		cfg.ProfilePath = defaultCuratorProfilePath
	}
	if abs, err := filepath.Abs(cfg.ProfilePath); err == nil {
		cfg.ProfilePath = abs
	}
	return cfg, nil
}
