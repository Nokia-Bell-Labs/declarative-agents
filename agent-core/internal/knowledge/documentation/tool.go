// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const defaultAddr = ":18081"
const (
	documentationServerLaunched = core.Signal("ServerLaunched")
	documentationServerStopped  = core.Signal("ServerStopped")
)

// ToolConfig configures the launch_documentation boundary word.
type ToolConfig struct {
	Addr        string `json:"addr"`
	DocsDir     string `json:"docs_dir"`
	ConfigsDir  string `json:"configs_dir"`
	SourceDir   string `json:"source_dir"`
	ProfilePath string `json:"profile_path"`
}

// RegisterFactories registers Knowledge Manager documentation builtin factories.
// launch_documentation and stop_documentation share one host lifecycle owner, so
// stop tears down exactly the listener launch started; each word names one forward
// operation for MachineSpec rather than one word branching on the prior signal
// (GH-508).
func RegisterFactories(br *toolregistry.BuiltinRegistry) {
	host := NewDocumentationHostLifecycle()
	br.Register("launch_documentation", LaunchDocumentationFactory(host))
	br.Register("stop_documentation", StopDocumentationFactory(host))
	RegisterRequestFactories(br)
}

// RegisterRequestFactories registers builtins used by documentation machine_request profiles.
func RegisterRequestFactories(br *toolregistry.BuiltinRegistry) {
	br.Register("doc_index_response", responseFactory("doc_index_response", "DocumentIndexReady"))
	br.Register("doc_detail_response", responseFactory("doc_detail_response", "DocumentDetailReady"))
}

// LaunchDocumentationFactory creates builders that start the documentation UI host
// on the shared lifecycle owner.
func LaunchDocumentationFactory(host *DocumentationHostLifecycle) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		cfg, err := decodeToolConfig(def)
		if err != nil {
			return nil, err
		}
		return launchDocumentationBuilder{config: cfg, host: host}, nil
	}
}

// StopDocumentationFactory creates builders that stop the shared documentation host.
func StopDocumentationFactory(host *DocumentationHostLifecycle) toolregistry.BuiltinFactory {
	return func(catalog.ToolDef, map[string]string) (core.Builder, error) {
		return stopDocumentationBuilder{host: host}, nil
	}
}

func responseFactory(name string, signal core.Signal) toolregistry.BuiltinFactory {
	return func(catalog.ToolDef, map[string]string) (core.Builder, error) {
		return responseBuilder{name: name, signal: signal}, nil
	}
}

// DocumentationHostLifecycle owns the documentation listener launch_documentation
// starts and stop_documentation stops.
type DocumentationHostLifecycle struct {
	mu      sync.Mutex
	running *RunningServer
}

// NewDocumentationHostLifecycle creates empty documentation host ownership state.
func NewDocumentationHostLifecycle() *DocumentationHostLifecycle {
	return &DocumentationHostLifecycle{}
}

func hostOrNew(host *DocumentationHostLifecycle) *DocumentationHostLifecycle {
	if host == nil {
		return NewDocumentationHostLifecycle()
	}
	return host
}

// launchDocumentationBuilder creates launch_documentation commands.
type launchDocumentationBuilder struct {
	config ToolConfig
	host   *DocumentationHostLifecycle
}

func (b launchDocumentationBuilder) Build(_ core.Result) core.Command {
	return launchDocumentationCmd{config: b.config, host: hostOrNew(b.host)}
}

type launchDocumentationCmd struct {
	config ToolConfig
	host   *DocumentationHostLifecycle
}

func (c launchDocumentationCmd) Name() string { return "launch_documentation" }

// Undo tears the host down, so a launch is compensated during a rollback (GH-508).
func (c launchDocumentationCmd) Undo(_ core.Result) core.Result {
	return stopDocumentationHost(c.host, c.Name())
}

func (c launchDocumentationCmd) Execute() core.Result {
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

// stopDocumentationBuilder creates stop_documentation commands.
type stopDocumentationBuilder struct {
	host *DocumentationHostLifecycle
}

func (b stopDocumentationBuilder) Build(_ core.Result) core.Command {
	return stopDocumentationCmd{host: hostOrNew(b.host)}
}

type stopDocumentationCmd struct {
	host *DocumentationHostLifecycle
}

func (c stopDocumentationCmd) Name() string { return "stop_documentation" }

// Undo is a noop: a stopped listener is re-established by a separate launch action.
func (c stopDocumentationCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c stopDocumentationCmd) Execute() core.Result {
	return stopDocumentationHost(c.host, c.Name())
}

// stopDocumentationHost stops the shared documentation host and reports the released address.
func stopDocumentationHost(host *DocumentationHostLifecycle, name string) core.Result {
	addr, err := host.Stop()
	if err != nil {
		return core.Result{Signal: core.CommandError, CommandName: name, Err: err, Output: err.Error()}
	}
	return core.Result{Signal: documentationServerStopped, CommandName: name, Output: jsonOutput(map[string]string{
		"signal": string(documentationServerStopped),
		"addr":   addr,
	})}
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
	if cfg.ProfilePath != "" {
		abs, err := filepath.Abs(cfg.ProfilePath)
		if err != nil {
			return ToolConfig{}, err
		}
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
