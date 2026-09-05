// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

const (
	InitClientGet    = "rest_client_get"
	InitClientSet    = "rest_client_set"
	InitClientCreate = "rest_client_create"
	InitClientDelete = "rest_client_delete"
	InitClientInvoke = "rest_client_invoke"
	InitClientSend   = "rest_client_send"
	InitClientAwait  = "rest_client_await"
	InitServerLaunch = "rest_server_launch"
	InitServerAwait  = "rest_server_await"
	InitServerStop   = "rest_server_stop"
	InitAwaitEvent   = "rest_await_event"
)

// StandardInits lists every REST builtin init name.
var StandardInits = []string{
	InitClientGet, InitClientSet, InitClientCreate, InitClientDelete, InitClientInvoke,
	InitClientSend, InitClientAwait, InitServerLaunch, InitServerAwait, InitServerStop,
	InitAwaitEvent,
}

// FactoryDeps holds REST factory dependencies.
type FactoryDeps struct {
	Definitions        Collection
	ServerState        *ServerState
	AsyncState         *AsyncState
	MachineRunner      MachineRequestRunner
	SignalSourceRunner SignalSourceRunner
	Monitor            MonitorState
	RunID              string
	CredentialResolver credentials.Resolver
}

// BuiltinRegistrar registers builtin factories for a selected tool set. The
// request registry is passed so factories that resolve tool vocabularies (for
// example parse_response and the $tool dispatch) bind to the request-local
// registry rather than the host agent's, and so per-request conversations stay
// isolated across machine_request runs.
type BuiltinRegistrar func(*toolregistry.BuiltinRegistry, map[string]bool, *core.Registry)

// ProfileMachineRequestRunnerDeps wires profile-backed machine_request runs.
type ProfileMachineRequestRunnerDeps struct {
	BaseDir          string
	Directory        string
	Vars             map[string]string
	RegisterBuiltins BuiltinRegistrar
	ExecBuilder      toolregistry.ExecBuilderFactory
}

// ProfileMachineRequestRunner runs request machines from trusted profile config.
type ProfileMachineRequestRunner struct {
	deps ProfileMachineRequestRunnerDeps
}

// NewProfileMachineRequestRunner creates a configured machine_request runner.
func NewProfileMachineRequestRunner(deps ProfileMachineRequestRunnerDeps) *ProfileMachineRequestRunner {
	return &ProfileMachineRequestRunner{deps: deps}
}

// RunMachineRequest loads the configured request profile and runs one machine.
func (r *ProfileMachineRequestRunner) RunMachineRequest(
	ctx context.Context,
	req MachineRequestRun,
) (MachineRequestResult, error) {
	if req.Config.MachineSpec != nil {
		return defaultMachineRequestRunner{}.RunMachineRequest(ctx, req)
	}
	cfg, err := r.prepareConfig(req.Config)
	if err != nil {
		return MachineRequestResult{}, err
	}
	req.Config = cfg
	return defaultMachineRequestRunner{}.RunMachineRequest(ctx, req)
}

func (r *ProfileMachineRequestRunner) prepareConfig(cfg restdef.MachineRequest) (restdef.MachineRequest, error) {
	profilePath, profile, err := r.loadRequestProfile(cfg)
	if err != nil {
		return restdef.MachineRequest{}, err
	}
	machinePath := requestMachinePath(cfg, profile, filepath.Dir(profilePath))
	machine, err := core.LoadMachineSpec(machinePath)
	if err != nil {
		return restdef.MachineRequest{}, fmt.Errorf("machine_config_invalid: load request machine: %w", err)
	}
	if err := core.ValidateRequiredMachinePolicy(machine); err != nil {
		return restdef.MachineRequest{}, fmt.Errorf("machine_config_invalid: request machine policy: %w", err)
	}
	if err := validateMachineResponses(machine, cfg.Response); err != nil {
		return restdef.MachineRequest{}, err
	}
	reg, err := r.requestRegistry(profilePath, profile, machine)
	if err != nil {
		return restdef.MachineRequest{}, err
	}
	cfg.MachineSpec = &machine
	cfg.Registry = reg
	// Wire dynamic $tool dispatch so a request-scoped machine can route to an
	// LLM-selected external word (for example the chatbot router picking a
	// chat-LLM word). Verbose is false, so the action returns the resolved
	// command directly, keeping command-state injection working on it.
	cfg.ToolAction = toolregistry.BuildDynamicToolAction(toolregistry.DynamicToolActionDeps{
		Registry: reg,
	})
	cfg.Budget = machine.BudgetSpec.ToBudget(core.Budget{})
	cfg.CommandTimeout = machine.BudgetSpec.CommandTimeout
	return cfg, nil
}

func (r *ProfileMachineRequestRunner) loadRequestProfile(
	cfg restdef.MachineRequest,
) (string, catalog.AgentProfile, error) {
	if cfg.Profile == "" {
		return "", catalog.AgentProfile{}, fmt.Errorf("machine_config_invalid: machine_request profile is required")
	}
	path := configuredPath(r.deps.BaseDir, cfg.Profile)
	profile, err := catalog.LoadProfile(path)
	if err != nil {
		return "", catalog.AgentProfile{}, fmt.Errorf("machine_config_invalid: load request profile: %w", err)
	}
	return path, profile, nil
}

func requestMachinePath(cfg restdef.MachineRequest, profile catalog.AgentProfile, profileDir string) string {
	if cfg.Machine == "" {
		return profile.Machine
	}
	return configuredPath(profileDir, cfg.Machine)
}

type declaredMachine struct {
	path    string
	machine core.MachineSpec
}

// LoadDeclaredMachines loads the root and each distinct machine_request
// declaration from one trusted profile closure. The root remains first; request
// machines are ordered by name so REST definition map iteration cannot affect
// the monitor response (srd033 R8).
func LoadDeclaredMachines(
	root core.MachineSpec,
	rootPath string,
	profileDir string,
	defs Collection,
) ([]core.MachineSpec, error) {
	seen := map[string]bool{canonicalMachinePath(rootPath): true}
	var declarations []declaredMachine
	for _, server := range defs.Servers {
		for _, endpoint := range server.Endpoints {
			if endpoint.Binding != bindingMachineRequest {
				continue
			}
			declaration, err := loadDeclaredMachine(endpoint.MachineRequest, profileDir)
			if err != nil {
				return nil, err
			}
			if seen[declaration.path] {
				continue
			}
			seen[declaration.path] = true
			declarations = append(declarations, declaration)
		}
	}
	sortDeclaredMachines(declarations)
	return prependRootMachine(root, declarations), nil
}

func loadDeclaredMachine(cfg restdef.MachineRequest, profileDir string) (declaredMachine, error) {
	if cfg.MachineSpec != nil {
		return declaredMachine{
			path: fmt.Sprintf("programmatic:%p", cfg.MachineSpec), machine: *cfg.MachineSpec,
		}, nil
	}
	profilePath := configuredPath(profileDir, cfg.Profile)
	profile, err := catalog.LoadProfile(profilePath)
	if err != nil {
		return declaredMachine{}, fmt.Errorf("load declared machine profile %s: %w", profilePath, err)
	}
	machinePath := canonicalMachinePath(requestMachinePath(cfg, profile, filepath.Dir(profilePath)))
	machine, err := core.LoadMachineSpec(machinePath)
	if err != nil {
		return declaredMachine{}, fmt.Errorf("load declared machine %s: %w", machinePath, err)
	}
	return declaredMachine{path: machinePath, machine: machine}, nil
}

func sortDeclaredMachines(declarations []declaredMachine) {
	sort.Slice(declarations, func(i, j int) bool {
		if declarations[i].machine.Name == declarations[j].machine.Name {
			return declarations[i].path < declarations[j].path
		}
		return declarations[i].machine.Name < declarations[j].machine.Name
	})
}

func prependRootMachine(root core.MachineSpec, declarations []declaredMachine) []core.MachineSpec {
	machines := make([]core.MachineSpec, 1, len(declarations)+1)
	machines[0] = root
	for _, declaration := range declarations {
		machines = append(machines, declaration.machine)
	}
	return machines
}

func canonicalMachinePath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func (r *ProfileMachineRequestRunner) requestRegistry(
	profilePath string,
	profile catalog.AgentProfile,
	machine core.MachineSpec,
) (*core.Registry, error) {
	defs, err := requestToolDefs(profile, machine)
	if err != nil {
		return nil, err
	}
	if err := catalog.ValidateToolEmits(machine, defs); err != nil {
		return nil, fmt.Errorf("machine_config_invalid: %w", err)
	}
	if err := catalog.ValidateToolPhases(machine, defs); err != nil {
		return nil, fmt.Errorf("machine_config_invalid: %w", err)
	}
	return r.registerRequestTools(profilePath, profile, machine, defs)
}

func requestToolDefs(profile catalog.AgentProfile, machine core.MachineSpec) ([]catalog.ToolDef, error) {
	dirDefs, err := catalog.LoadToolDeclarationsFromDirs(profile.ToolConfigDirs)
	if err != nil {
		return nil, fmt.Errorf("machine_config_invalid: load request tool config dirs: %w", err)
	}
	fileDefs, err := catalog.LoadToolDeclarations(profile.ToolDeclarations)
	if err != nil {
		return nil, fmt.Errorf("machine_config_invalid: load request tool declarations: %w", err)
	}
	merged := catalog.MergeToolDefs(dirDefs, fileDefs)
	selection := machineActionNames(machine)
	if machineHasDynamicDispatch(machine) {
		// A $tool transition dispatches an LLM-selected external word that may not
		// appear as a literal transition action. Restrict that vocabulary to the
		// trusted profile's tools selection: declaration directories can contain
		// unrelated external words whose signals the selected machine does not
		// handle (for example generic document-resource words beside a coding
		// executor's filesystem tools).
		profileSelection, err := catalog.LoadToolSelections(profile.Tools)
		if err != nil {
			return nil, fmt.Errorf("machine_config_invalid: load request tool selection: %w", err)
		}
		seen := make(map[string]bool, len(selection))
		for _, name := range selection {
			seen[name] = true
		}
		for _, name := range selectedDynamicDispatchVocabulary(merged, profileSelection) {
			if !seen[name] {
				seen[name] = true
				selection = append(selection, name)
			}
		}
	}
	defs, err := catalog.SelectTools(merged, selection)
	if err != nil {
		return nil, fmt.Errorf("machine_config_invalid: select request tools: %w", err)
	}
	return defs, nil
}

func selectedDynamicDispatchVocabulary(defs []catalog.ToolDef, selected []string) []string {
	allowed := make(map[string]bool, len(selected))
	for _, name := range selected {
		allowed[name] = true
	}
	var vocabulary []string
	for _, name := range dynamicDispatchVocabulary(defs) {
		if allowed[name] {
			vocabulary = append(vocabulary, name)
		}
	}
	return vocabulary
}

// machineHasDynamicDispatch reports whether any transition dispatches via $tool.
func machineHasDynamicDispatch(machine core.MachineSpec) bool {
	for _, transition := range machine.Transitions {
		if transition.Action == "$tool" {
			return true
		}
	}
	return false
}

// dynamicDispatchVocabulary returns the names of the external-visibility tool
// defs, the vocabulary a $tool transition may dispatch.
func dynamicDispatchVocabulary(defs []catalog.ToolDef) []string {
	var names []string
	for _, def := range defs {
		if def.Visibility != "internal" {
			names = append(names, def.Name)
		}
	}
	return names
}

func (r *ProfileMachineRequestRunner) registerRequestTools(
	profilePath string,
	profile catalog.AgentProfile,
	machine core.MachineSpec,
	defs []catalog.ToolDef,
) (*core.Registry, error) {
	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	selected := toolregistry.SelectedBuiltinInits(defs)
	if r.deps.RegisterBuiltins != nil {
		r.deps.RegisterBuiltins(builtins, selected, reg)
	}
	vars := r.requestVars(profilePath, profile)
	if err := toolregistry.RegisterUnifiedToolsForMachine(reg, builtins, vars["directory"], machine, defs, vars, r.deps.ExecBuilder); err != nil {
		return nil, fmt.Errorf("machine_config_invalid: register request tools: %w", err)
	}
	return reg, nil
}

func (r *ProfileMachineRequestRunner) requestVars(profilePath string, profile catalog.AgentProfile) map[string]string {
	vars := map[string]string{}
	for name, value := range r.deps.Vars {
		vars[name] = value
	}
	if vars["directory"] == "" {
		vars["directory"] = requestDirectory(r.deps.Directory, profile, profilePath)
	}
	return vars
}

func requestDirectory(configured string, profile catalog.AgentProfile, profilePath string) string {
	switch {
	case configured != "":
		return configured
	case profile.Directory != "":
		return profile.Directory
	default:
		return filepath.Dir(profilePath)
	}
}

func machineActionNames(machine core.MachineSpec) []string {
	seen := map[string]bool{}
	var names []string
	for _, transition := range machine.Transitions {
		if transition.Action == "" || transition.Action == "$tool" || seen[transition.Action] {
			continue
		}
		seen[transition.Action] = true
		names = append(names, transition.Action)
	}
	return names
}

// validateMachineResponses rejects a response map keyed on something the
// machine cannot produce. A mapped signal must be declared, and a mapped state
// must be declared terminal -- a non-terminal state never ends a run, so a
// mapping onto one is dead configuration that would surface as a
// response_missing at request time instead of at load (srd030 R2.5; GH-615).
func validateMachineResponses(machine core.MachineSpec, response restdef.MachineRequestResponse) error {
	signals := map[string]bool{}
	for _, signal := range machine.Signals.Names() {
		signals[signal] = true
	}
	for signal := range response.TerminalSignals {
		if !signals[signal] {
			return fmt.Errorf("machine_config_invalid: response terminal signal %q is not declared", signal)
		}
	}
	terminals := map[string]bool{}
	for _, state := range machine.TerminalStates {
		terminals[state] = true
	}
	for state := range response.TerminalStates {
		if !terminals[state] {
			return fmt.Errorf("machine_config_invalid: response terminal state %q is not a terminal state of machine %q",
				state, machine.Name)
		}
	}
	return nil
}

func configuredPath(base, path string) string {
	if filepath.IsAbs(path) || base == "" {
		return path
	}
	return filepath.Join(base, path)
}

// RegisterFactories registers REST builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	if deps.ServerState == nil {
		deps.ServerState = NewServerState()
	}
	if deps.AsyncState == nil {
		deps.AsyncState = NewAsyncState()
	}
	for _, init := range StandardInits {
		br.Register(init, factoryFor(init, deps))
	}
}

func factoryFor(init string, deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		switch init {
		case InitServerLaunch, InitServerAwait, InitServerStop:
			return newServerBuilder(def, init, deps)
		case InitAwaitEvent:
			return newAwaitEventBuilder(def, deps)
		default:
			return newClientBuilder(def, init, deps)
		}
	}
}

func newClientBuilder(def catalog.ToolDef, init string, deps FactoryDeps) (core.Builder, error) {
	var cfg ClientToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if err := validateClientToolConfig(def.Name, cfg); err != nil {
		return nil, err
	}
	operation, err := deps.Definitions.ResolveClientOperation(cfg)
	if err != nil {
		return nil, err
	}
	if init == InitClientSend && operation.Operation.Async == nil {
		return nil, fmt.Errorf("tool %q requires async REST operation", def.Name)
	}
	if err := validateClientRollbackPolicy(def, init, operation); err != nil {
		return nil, err
	}
	if err := validateClientEmits(def, init, operation); err != nil {
		return nil, err
	}
	return ClientBuilder{
		ToolName: def.Name, Init: init, Operation: operation, Definitions: deps.Definitions,
		AsyncState: deps.AsyncState, Credentials: deps.CredentialResolver, Metrics: def.Metrics,
	}, nil
}

func validateClientRollbackPolicy(
	def catalog.ToolDef,
	init string,
	operation ClientOperationDefinition,
) error {
	toolExpectsReceipt := def.Reversibility.Classification == "compensatable"
	operationProducesReceipt := init != InitClientAwait &&
		operation.Operation.Reversibility.Classification == "compensatable" &&
		len(operation.Operation.Compensation) > 0
	if toolExpectsReceipt == operationProducesReceipt {
		return nil
	}
	if toolExpectsReceipt {
		return fmt.Errorf(
			"tool %q is compensatable but REST operation %q cannot produce a rollback receipt",
			def.Name, operation.OperationName,
		)
	}
	return fmt.Errorf(
		"tool %q rollback policy %q disagrees with receipt-producing REST operation %q",
		def.Name, def.Reversibility.Classification, operation.OperationName,
	)
}

func newServerBuilder(def catalog.ToolDef, init string, deps FactoryDeps) (core.Builder, error) {
	var cfg ServerToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if cfg.RestRef == "" {
		return nil, fmt.Errorf("tool %q config requires rest_ref", def.Name)
	}
	server, err := deps.Definitions.ResolveServer(cfg.RestRef)
	if err != nil {
		return nil, err
	}
	server.MachineRequestRunner = deps.MachineRunner
	server.SignalSourceRunner = deps.SignalSourceRunner
	server.Monitor = deps.Monitor
	server.RunID = deps.RunID
	server.Credentials = deps.CredentialResolver
	if init == InitServerAwait {
		if err := validateServerAwaitEmits(def, server); err != nil {
			return nil, err
		}
	}
	return ServerBuilder{ToolName: def.Name, Init: init, Server: server, State: deps.ServerState}, nil
}

func newAwaitEventBuilder(def catalog.ToolDef, deps FactoryDeps) (core.Builder, error) {
	var cfg AwaitEventToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	options, err := awaitAnyOptions(def.Name, cfg, deps.Definitions)
	if err != nil {
		return nil, err
	}
	if err := validateAwaitAnyEmits(def, options, deps.Definitions); err != nil {
		return nil, err
	}
	return AwaitEventBuilder{ToolName: def.Name, Options: options, State: deps.ServerState}, nil
}

func validateClientToolConfig(toolName string, cfg ClientToolConfig) error {
	if cfg.RestRef == "" {
		return fmt.Errorf("tool %q config requires rest_ref", toolName)
	}
	if cfg.Operation == "" {
		return fmt.Errorf("tool %q config requires operation", toolName)
	}
	return nil
}

func awaitAnyOptions(toolName string, cfg AwaitEventToolConfig, defs Collection) (AwaitAnyOptions, error) {
	if len(cfg.Sources) == 0 {
		return AwaitAnyOptions{}, fmt.Errorf("tool %q config requires sources", toolName)
	}
	timeout, err := parseAwaitTimeout(toolName, cfg.Timeout)
	if err != nil {
		return AwaitAnyOptions{}, err
	}
	stopped, err := stoppedSourceBehavior(toolName, cfg.StoppedBehavior)
	if err != nil {
		return AwaitAnyOptions{}, err
	}
	if err := validateReadPolicy(toolName, cfg.ReadPolicy); err != nil {
		return AwaitAnyOptions{}, err
	}
	options := AwaitAnyOptions{Timeout: timeout, StoppedBehavior: stopped}
	for _, source := range cfg.Sources {
		awaitSource, err := awaitSourceConfig(toolName, source, cfg.AllowedSignals, defs)
		if err != nil {
			return AwaitAnyOptions{}, err
		}
		options.Sources = append(options.Sources, awaitSource)
	}
	return options, nil
}

func awaitSourceConfig(
	toolName string,
	cfg AwaitEventSourceConfig,
	allowedSignals []string,
	defs Collection,
) (AwaitSource, error) {
	if cfg.Server == "" {
		return AwaitSource{}, fmt.Errorf("tool %q source requires server", toolName)
	}
	if _, err := defs.ResolveServer(cfg.Server); err != nil {
		return AwaitSource{}, err
	}
	signals, err := signalFilters(toolName, cfg.Signals, allowedSignals)
	if err != nil {
		return AwaitSource{}, err
	}
	stopped, err := stoppedSourceBehavior(toolName, cfg.StoppedBehavior)
	if err != nil {
		return AwaitSource{}, err
	}
	return AwaitSource{
		Server: cfg.Server, Routes: cfg.Routes,
		Signals:         signals,
		StoppedBehavior: stopped,
	}, nil
}

func parseAwaitTimeout(toolName, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("tool %q config has invalid timeout %q", toolName, value)
	}
	return timeout, nil
}

func validateReadPolicy(toolName, value string) error {
	switch value {
	case "", "first_available":
		return nil
	default:
		return fmt.Errorf("tool %q config has unsupported read_policy %q", toolName, value)
	}
}

func signalFilters(toolName string, source, allowed []string) ([]string, error) {
	if len(source) == 0 || len(allowed) == 0 {
		return mergeSignals(source, allowed), nil
	}
	signals := intersectSignals(source, allowed)
	if len(signals) == 0 {
		return nil, fmt.Errorf("tool %q source signals do not match allowed_signals", toolName)
	}
	return signals, nil
}

func mergeSignals(source, allowed []string) []string {
	if len(source) > 0 {
		return source
	}
	return allowed
}

func intersectSignals(source, allowed []string) []string {
	seen := map[string]bool{}
	for _, signal := range allowed {
		seen[signal] = true
	}
	var signals []string
	for _, signal := range source {
		if seen[signal] {
			signals = append(signals, signal)
		}
	}
	return signals
}

func stoppedSourceBehavior(toolName, value string) (StoppedSourceBehavior, error) {
	switch value {
	case "":
		return "", nil
	case string(StoppedSourceIgnore):
		return StoppedSourceIgnore, nil
	case string(StoppedSourceCommandError):
		return StoppedSourceCommandError, nil
	case string(StoppedSourceEmitServerStopped):
		return StoppedSourceEmitServerStopped, nil
	default:
		return "", fmt.Errorf("tool %q config has unsupported stopped_behavior %q", toolName, value)
	}
}

// LoadDeclaredTools loads every tool declaration of the trusted profile
// closure as authored (srd033 R9): the root profile's tool_declarations files
// and those of every profile a machine_request endpoint references. Entries
// are the raw declaration mappings -- fields the runtime does not interpret
// ride through -- ordered by word name; a file shared across profiles
// contributes each word once, and a name declared twice keeps the first in
// file-resolution order.
func LoadDeclaredTools(rootProfilePath, profileDir string, defs Collection) ([]map[string]interface{}, error) {
	paths, err := declaredToolPaths(rootProfilePath, profileDir, defs)
	if err != nil {
		return nil, err
	}
	seenWords := map[string]bool{}
	var declarations []map[string]interface{}
	for _, path := range paths {
		words, err := readDeclaredToolWords(path)
		if err != nil {
			return nil, err
		}
		for _, word := range words {
			name, _ := word["name"].(string)
			if name == "" || seenWords[name] {
				continue
			}
			seenWords[name] = true
			declarations = append(declarations, word)
		}
	}
	sort.Slice(declarations, func(i, j int) bool {
		left, _ := declarations[i]["name"].(string)
		right, _ := declarations[j]["name"].(string)
		return left < right
	})
	return declarations, nil
}

// declaredToolPaths resolves the closure's tool_declarations files: the root
// profile first, then each machine_request profile, each file once.
func declaredToolPaths(rootProfilePath, profileDir string, defs Collection) ([]string, error) {
	paths := []string{}
	seenFiles := map[string]bool{}
	appendProfileDeclarations := func(profilePath string) error {
		profile, err := catalog.LoadProfile(profilePath)
		if err != nil {
			return fmt.Errorf("load declared tools profile %s: %w", profilePath, err)
		}
		for _, declarationPath := range profile.ToolDeclarations {
			resolved := filepath.Clean(declarationPath)
			if seenFiles[resolved] {
				continue
			}
			seenFiles[resolved] = true
			paths = append(paths, resolved)
		}
		return nil
	}
	if err := appendProfileDeclarations(rootProfilePath); err != nil {
		return nil, err
	}
	for _, server := range defs.Servers {
		for _, endpoint := range server.Endpoints {
			if endpoint.Binding != bindingMachineRequest || endpoint.MachineRequest.Profile == "" {
				continue
			}
			if err := appendProfileDeclarations(configuredPath(profileDir, endpoint.MachineRequest.Profile)); err != nil {
				return nil, err
			}
		}
	}
	return paths, nil
}

// readDeclaredToolWords parses one declarations file as raw mappings and
// returns every list entry carrying a name, in file order.
func readDeclaredToolWords(path string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read declared tools %s: %w", path, err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse declared tools %s: %w", path, err)
	}
	keys := make([]string, 0, len(doc))
	for key := range doc {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var words []map[string]interface{}
	for _, key := range keys {
		list, ok := doc[key].([]interface{})
		if !ok {
			continue
		}
		for _, entry := range list {
			if word, ok := entry.(map[string]interface{}); ok {
				words = append(words, word)
			}
		}
	}
	return words, nil
}
