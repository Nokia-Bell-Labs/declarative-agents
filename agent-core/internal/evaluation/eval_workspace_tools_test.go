// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

func TestPointWorkspaceToolsPrepareWorkspaceSequence(t *testing.T) {
	pc := pointWorkspaceFixture(t)

	requireSignal(t, (&createPointDirCmd{pc: pc}).Execute(), SigPointDirCreated)
	require.DirExists(t, pc.PointDir)
	require.Equal(t, filepath.Join(pc.PointDir, ArtifactTrace), pc.TracePath)

	requireSignal(t, (&copySampleWorkspaceCmd{pc: pc}).Execute(), SigSampleWorkspaceCopied)
	require.FileExists(t, filepath.Join(pc.PointDir, "main.go"))

	requireSignal(t, (&copySampleDocsCmd{pc: pc}).Execute(), SigSampleDocsCopied)
	require.FileExists(t, filepath.Join(pc.PointDir, ArtifactDocDir, "README.md"))

	requireSignal(t, (&initWorkspaceRepoCmd{pc: pc}).Execute(), SigWorkspaceRepoInitialized)
	require.DirExists(t, filepath.Join(pc.PointDir, ".git"))

	requireSignal(t, (&stageWorkspaceBaselineCmd{pc: pc}).Execute(), SigWorkspaceBaselineStaged)

	requireSignal(t, (&commitWorkspaceBaselineCmd{pc: pc}).Execute(), SigWorkspaceBaselineCommitted)
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

	res := (&copySampleWorkspaceCmd{pc: pc}).Execute()
	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "point dir not initialized")

	requireSignal(t, (&createPointDirCmd{pc: pc}).Execute(), SigPointDirCreated)
	pc.Sample.WorkspaceDir = filepath.Join(t.TempDir(), "missing")
	res = (&copySampleWorkspaceCmd{pc: pc}).Execute()
	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "copy workspace")
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
