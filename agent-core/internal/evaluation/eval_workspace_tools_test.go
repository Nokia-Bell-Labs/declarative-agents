// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
)

func TestPointWorkspaceToolsPrepareWorkspaceSequence(t *testing.T) {
	pc := pointWorkspaceFixture(t)

	createResult := (&createPointDirCmd{pc: pc}).Execute()
	requireSignal(t, createResult, SigPointDirCreated)
	require.DirExists(t, pc.PointDir)
	require.Equal(t, filepath.Join(pc.PointDir, ArtifactTrace), pc.TracePath)

	requireSignal(t, executePointWord(t, pc, "copy_dir", createResult), core.ToolDone)
	require.FileExists(t, filepath.Join(pc.PointDir, "main.go"))

	requireSignal(t, (&copySampleDocsCmd{pc: pc}).Execute(), SigSampleDocsCopied)
	require.FileExists(t, filepath.Join(pc.PointDir, ArtifactDocDir, "README.md"))

	requireSignal(t, executePointWord(t, pc, "git_init", core.Result{}), core.ToolDone)
	require.DirExists(t, filepath.Join(pc.PointDir, ".git"))

	requireSignal(t, executePointWord(t, pc, "stage_all", core.Result{}), core.ToolDone)
	requireSignal(t, executePointDef(pc, baselineCommitDef(), core.Result{}), core.ToolDone)
	out, err := exec.Command("git", "-C", pc.PointDir, "log", "--oneline", "-1").CombinedOutput()
	require.NoError(t, err, string(out))
	require.Contains(t, string(out), "baseline")

	status, err := exec.Command("git", "-C", pc.PointDir, "status", "--porcelain").CombinedOutput()
	require.NoError(t, err, string(status))
	require.Empty(t, strings.TrimSpace(string(status)))
}

func TestCopySampleDocsNoopsWhenSampleHasNoDocs(t *testing.T) {
	pc := pointWorkspaceFixture(t)
	pc.Sample.DocDir = ""
	requireSignal(t, (&createPointDirCmd{pc: pc}).Execute(), SigPointDirCreated)

	res := (&copySampleDocsCmd{pc: pc}).Execute()

	requireSignal(t, res, SigSampleDocsCopied)
	require.Contains(t, res.Output, "no docs")
	require.NoDirExists(t, filepath.Join(pc.PointDir, ArtifactDocDir))
}

func TestPointWorkspaceToolsFailAtSplitBoundaries(t *testing.T) {
	pc := pointWorkspaceFixture(t)
	pc.Sample.WorkspaceDir = filepath.Join(t.TempDir(), "missing")
	createResult := (&createPointDirCmd{pc: pc}).Execute()

	res := executePointWord(t, pc, "copy_dir", createResult)
	require.Equal(t, core.ToolFailed, res.Signal)
}

func pointWorkspaceFixture(t *testing.T) *PointContext {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "sample", "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644))

	docDir := filepath.Join(root, "sample", "doc")
	require.NoError(t, os.MkdirAll(docDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(docDir, "README.md"), []byte("docs\n"), 0o644))

	return &PointContext{
		SessionDir: filepath.Join(root, "session"),
		PointID:    "sample-agent-model-rep0",
		Sample: Sample{
			Name:         "sample",
			WorkspaceDir: workspace,
			DocDir:       docDir,
		},
	}
}

func requireSignal(t *testing.T, res core.Result, signal core.Signal) {
	t.Helper()
	require.Equal(t, signal, res.Signal, res.Output)
	require.NoError(t, res.Err)
}

func executePointWord(t *testing.T, pc *PointContext, name string, prior core.Result) core.Result {
	t.Helper()
	defs, err := catalog.LoadToolDefs(filepath.Join("..", "..", "tools", "exec", "all.yaml"))
	require.NoError(t, err)
	for _, def := range defs {
		if def.Name == name {
			return executePointDef(pc, def, prior)
		}
	}
	t.Fatalf("shared exec word %q is not declared", name)
	return core.Result{}
}

func executePointDef(pc *PointContext, def catalog.ToolDef, prior core.Result) core.Result {
	builder := &toolexec.ExecBuilder{Def: def, RootFunc: func() string { return pc.PointDir }}
	return builder.Build(prior).Execute()
}

func baselineCommitDef() catalog.ToolDef {
	return catalog.ToolDef{
		Name:   "commit_workspace_baseline",
		Type:   "exec",
		Binary: "git",
		Args: []string{
			"-c", "user.name=agent-core",
			"-c", "user.email=agent-core@example.invalid",
			"commit", "-m", "baseline", "--allow-empty",
		},
		Precondition: "git_repo",
	}
}
