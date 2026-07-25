// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestListFilesDeclarationUsesBoundedFindExec(t *testing.T) {
	def := loadListFilesDef(t)

	require.Equal(t, "exec", def.Type)
	require.Equal(t, "bash", def.Binary)
	require.Contains(t, strings.Join(def.Args, "\n"), `find -P "$start" -mindepth 1 -maxdepth "$depth" -print`)
	require.Equal(t, DefaultOutputLineCap, def.OutputCap)

	cmd := (&ExecBuilder{Def: def, Root: t.TempDir()}).Build(
		core.Result{Output: `{"parameters":{"max_depth":2}}`},
	)
	require.Equal(t, "2", cmd.(*ExecCmd).params["max_depth"])
}

func TestListFilesExecPreservesTreeContract(t *testing.T) {
	requireGNUFind(t)
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "a"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a", "deep.txt"), []byte("deep"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a-file"), []byte("peer"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".hidden"), []byte("hidden"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "z"), []byte("last"), 0o644))
	require.NoError(t, os.Symlink("a", filepath.Join(root, "link")))

	res := executeListFiles(t, root, `{}`)

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Equal(t, ".hidden\na/\n  a/deep.txt\na-file\nlink\nz", res.Output)
	require.NotContains(t, res.Output, "link/deep.txt", "directory symlinks must not be followed")
}

func TestListFilesExecHonorsDepthAndSubpath(t *testing.T) {
	requireGNUFind(t)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a", "b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a", "b", "deep.txt"), []byte("x"), 0o644))

	shallow := executeListFiles(t, root, `{"max_depth":1}`)
	require.Equal(t, core.ToolDone, shallow.Signal, shallow.Output)
	require.Equal(t, "a/", shallow.Output)

	scoped := executeListFiles(t, root, `{"path":"a","max_depth":2}`)
	require.Equal(t, core.ToolDone, scoped.Signal, scoped.Output)
	require.Equal(t, "a/b/\n  a/b/deep.txt", scoped.Output)
}

func TestListFilesExecRejectsMissingAndEscapingPaths(t *testing.T) {
	requireGNUFind(t)
	root := t.TempDir()

	missing := executeListFiles(t, root, `{"path":"missing"}`)
	require.Equal(t, core.ToolFailed, missing.Signal)
	require.Contains(t, missing.Output, "path not found: missing")

	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "escape")))
	escaping := executeListFiles(t, root, `{"path":"escape"}`)
	require.Equal(t, core.ToolFailed, escaping.Signal)
	require.Contains(t, escaping.Output, "outside the workspace")
}

func TestListFilesExecCapsOutput(t *testing.T) {
	requireGNUFind(t)
	root := t.TempDir()
	for i := 0; i < DefaultOutputLineCap+5; i++ {
		name := filepath.Join(root, fmt.Sprintf("file-%03d", i))
		require.NoError(t, os.WriteFile(name, nil, 0o644))
	}

	res := executeListFiles(t, root, `{}`)

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Contains(t, res.Output, "... 5 lines omitted")
	require.Contains(t, res.Output, "file-199")
	require.NotContains(t, res.Output, "file-200")
}

func executeListFiles(t *testing.T, root, parameters string) core.Result {
	t.Helper()
	def := loadListFilesDef(t)
	request := fmt.Sprintf(`{"parameters":%s}`, parameters)
	return (&ExecBuilder{Def: def, Root: root}).Build(core.Result{Output: request}).Execute()
}

func loadListFilesDef(t *testing.T) catalog.ToolDef {
	t.Helper()
	path := filepath.Join("..", "..", "..", "tools", "builtin", "list-files.yaml")
	defs, err := catalog.LoadToolDefs(path)
	require.NoError(t, err)
	require.Len(t, defs, 1)
	return defs[0]
}

func requireGNUFind(t *testing.T) {
	t.Helper()
	cmd := osexec.Command("find", "-P", ".", "-mindepth", "0", "-maxdepth", "0", "-print")
	cmd.Dir = t.TempDir()
	if err := cmd.Run(); err != nil {
		t.Skip("list_files runtime requires GNU find (findutils)")
	}
}
