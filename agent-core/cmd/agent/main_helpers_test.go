// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
	"github.com/stretchr/testify/require"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	spec.SetAgentCoreInstallRoot(repoRootFromRuntime())
	os.Exit(m.Run())
}

func assertMainDeclsAbsent(t *testing.T, forbidden map[string]bool) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(currentFile), "main.go"), nil, 0)
	require.NoError(t, err)
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			require.False(t, forbidden[d.Name.Name], "main.go must not declare %s", d.Name.Name)
		case *ast.GenDecl:
			assertGenDeclNamesAbsent(t, d, forbidden)
		}
	}
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func requireToolDef(t *testing.T, defs []catalog.ToolDef, name string) catalog.ToolDef {
	t.Helper()
	for _, def := range defs {
		if def.Name == name {
			return def
		}
	}
	require.Failf(t, "missing tool definition", "tool %q not found", name)
	return catalog.ToolDef{}
}

func (b staticSignalBuilder) Build(previous core.Result) core.Command {
	// Once the machine begins tearing down, a launch tool re-dispatched during
	// shutdown emits its afterExit signal instead of its launch signal. Teardown
	// can span several steps (e.g. exit_agent -> stop_monitor_rest ->
	// serve_documentation in the documentation-curator machine), so the switch
	// fires on any teardown-phase signal, not just the AgentExited that opens it.
	if b.afterExit != "" && isTeardownSignal(previous.Signal) {
		return staticSignalCmd{name: b.name, signal: b.afterExit, output: b.output}
	}
	return staticSignalCmd{name: b.name, signal: b.signal, output: b.output}
}

func (c staticSignalCmd) Name() string { return c.name }

func (c staticSignalCmd) Execute() core.Result {
	return core.Result{CommandName: c.name, Signal: c.signal, Output: c.output}
}

func (c staticSignalCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.name)
}

func assertGenDeclNamesAbsent(t *testing.T, decl *ast.GenDecl, forbidden map[string]bool) {
	t.Helper()
	for _, spec := range decl.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range value.Names {
			require.False(t, forbidden[name.Name], "main.go must not declare %s", name.Name)
		}
	}
}

type agentFlagSnapshot struct {
	profile          string
	coreRoot         string
	otelLog          string
	otelParent       string
	directory        string
	verboseTrace     bool
	request          string
	output           string
	resumeCheckpoint string
	resumeSignal     string
	validateConfig   bool
}

func snapshotAgentFlags() agentFlagSnapshot {
	return agentFlagSnapshot{
		profile:          flagProfile,
		coreRoot:         flagCoreRoot,
		otelLog:          flagOTelLog,
		otelParent:       flagOTelParent,
		directory:        flagDirectory,
		verboseTrace:     flagVerboseTrace,
		request:          flagRequest,
		output:           flagOutput,
		resumeCheckpoint: flagResumeCheckpoint,
		resumeSignal:     flagResumeSignal,
		validateConfig:   flagValidateConfig,
	}
}

func restoreAgentFlags(s agentFlagSnapshot) {
	flagProfile = s.profile
	flagCoreRoot = s.coreRoot
	flagOTelLog = s.otelLog
	flagOTelParent = s.otelParent
	flagDirectory = s.directory
	flagVerboseTrace = s.verboseTrace
	flagRequest = s.request
	flagOutput = s.output
	flagResumeCheckpoint = s.resumeCheckpoint
	flagResumeSignal = s.resumeSignal
	flagValidateConfig = s.validateConfig
}

func clearAgentFlags() {
	restoreAgentFlags(agentFlagSnapshot{resumeSignal: string(core.Approved)})
}

func repoRootFromTest(t *testing.T) string {
	t.Helper()
	return repoRootFromRuntime()
}

func repoRootFromRuntime() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

func profileRootFromTest(t *testing.T) string {
	t.Helper()
	root := repoRootFromTest(t)
	for _, candidate := range profileRootCandidates(root) {
		if hasTestProfile(candidate, "executor") || hasTestProfile(candidate, "monitor") {
			return candidate
		}
		nested := filepath.Join(candidate, "agents")
		if hasTestProfile(nested, "executor") || hasTestProfile(nested, "monitor") {
			return nested
		}
	}
	t.Fatalf("profile root not found; place agent-profiles next to agent-core or under ./agent-profiles")
	return ""
}

func profileRootCandidates(root string) []string {
	return []string{
		filepath.Join(filepath.Dir(root), "agent-profiles"),
		filepath.Join(root, "agent-profiles"),
	}
}

func hasTestProfile(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel), "profile.yaml"))
	return err == nil
}

func profilePathFromTest(t *testing.T, rel string) string {
	t.Helper()
	return profilePathFromRoot(profileRootFromTest(t), rel)
}

func profilePathFromRoot(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	defer func() { os.Stderr = old }()

	runErr := fn()
	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, readErr := buf.ReadFrom(r)
	require.NoError(t, readErr)
	require.NoError(t, r.Close())
	return buf.String(), runErr
}
