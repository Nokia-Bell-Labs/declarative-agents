// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
)

const defaultAddr = ":18081"
const (
	documentationServerLaunched = core.Signal("ServerLaunched")
	documentationServerStopped  = core.Signal("ServerStopped")
)

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
		return ServeDocumentationBuilder{Config: cfg, Host: NewDocumentationHostLifecycle()}, nil
	}
}

// ServeDocumentationBuilder creates serve_documentation commands.
type ServeDocumentationBuilder struct {
	Config ToolConfig
	Host   *DocumentationHostLifecycle
}

// DocumentationHostLifecycle owns the serve_documentation listener.
type DocumentationHostLifecycle struct {
	mu      sync.Mutex
	running *RunningServer
}

// NewDocumentationHostLifecycle creates empty documentation host ownership state.
func NewDocumentationHostLifecycle() *DocumentationHostLifecycle {
	return &DocumentationHostLifecycle{}
}

func (b ServeDocumentationBuilder) Build(previous core.Result) core.Command {
	host := b.Host
	if host == nil {
		host = NewDocumentationHostLifecycle()
	}
	return serveDocumentationCmd{
		config: b.Config, host: host,
		stop: previous.Signal == core.Signal("AgentExited"),
	}
}

type serveDocumentationCmd struct {
	config ToolConfig
	host   *DocumentationHostLifecycle
	stop   bool
}

func (c serveDocumentationCmd) Name() string { return "serve_documentation" }

func (c serveDocumentationCmd) Undo() core.Result {
	return c.stopHost()
}

func (c serveDocumentationCmd) UndoMemento() (core.UndoMemento, error) {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy:   "server_shutdown",
		ServerAddr: c.hostAddr(),
		Requires:   []string{"addr"},
	}}
	return undo.BoundaryCompensationMemento(c.Name(), payload, "stop the owned documentation host listener")
}

func (c serveDocumentationCmd) Execute() core.Result {
	if c.stop {
		return c.stopHost()
	}
	running, err := c.host.Start(HostConfig{
		Addr:        c.config.Addr,
		DocsDir:     c.config.DocsDir,
		ConfigsDir:  c.config.ConfigsDir,
		SourceDir:   c.config.SourceDir,
		ProfilePath: c.config.ProfilePath,
		Workflow:    NewLazyProfileWorkflowRunner(c.config.ProfilePath, c.config.DocsDir),
	})
	if err != nil {
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Err: err, Output: err.Error()}
	}
	return core.Result{Signal: documentationServerLaunched, CommandName: c.Name(), Output: jsonOutput(map[string]string{
		"signal": string(documentationServerLaunched),
		"addr":   running.Addr,
	})}
}

func (c serveDocumentationCmd) stopHost() core.Result {
	addr, err := c.host.Stop()
	if err != nil {
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Err: err, Output: err.Error()}
	}
	return core.Result{Signal: documentationServerStopped, CommandName: c.Name(), Output: jsonOutput(map[string]string{
		"signal": string(documentationServerStopped),
		"addr":   addr,
	})}
}

func (c serveDocumentationCmd) hostAddr() string {
	if c.host == nil {
		return c.config.Addr
	}
	if addr := c.host.Addr(); addr != "" {
		return addr
	}
	return c.config.Addr
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

// Start launches and retains the owned documentation host.
func (h *DocumentationHostLifecycle) Start(cfg HostConfig) (*RunningServer, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running != nil {
		return nil, fmt.Errorf("documentation host already running at %s", h.running.Addr)
	}
	running, err := NewServer(cfg).Start()
	if err != nil {
		return nil, err
	}
	h.running = running
	return running, nil
}

// Stop stops and clears the owned documentation host.
func (h *DocumentationHostLifecycle) Stop() (string, error) {
	h.mu.Lock()
	running := h.running
	h.running = nil
	h.mu.Unlock()
	if running == nil {
		return "", fmt.Errorf("documentation host is not running")
	}
	return running.Addr, running.Stop()
}

// Addr returns the current owned host address.
func (h *DocumentationHostLifecycle) Addr() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running == nil {
		return ""
	}
	return h.running.Addr
}

func jsonOutput(output map[string]string) string {
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}
