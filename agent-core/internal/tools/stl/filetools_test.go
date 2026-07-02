// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

func toolReq(params string) core.Result {
	return core.Result{Output: `{"parameters":` + params + `}`}
}

func TestReadBuilder_MissingParam(t *testing.T) {
	t.Parallel()

	b := &ReadBuilder{Root: t.TempDir()}
	cmd := b.Build(toolReq(`{}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "missing required parameter")
}

func TestRead_Success(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "hello.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))

	b := &ReadBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"hello.go"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "package main")
	assert.Contains(t, res.Output, "1|")
}

func TestRead_LineRange(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "lines.txt"), []byte("a\nb\nc\nd\ne\n"), 0o644))

	b := &ReadBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"lines.txt","start_line":2,"end_line":4}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "2|b")
	assert.Contains(t, res.Output, "4|d")
	assert.NotContains(t, res.Output, "1|a")
}

func TestRead_Directory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "subdir"), 0o755))

	b := &ReadBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"subdir"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "directory")
}

func TestRead_Binary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "bin"), []byte{0x00, 0x01, 0x02}, 0o644))

	b := &ReadBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"bin"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "binary")
}

func TestRead_NotFound(t *testing.T) {
	t.Parallel()

	b := &ReadBuilder{Root: t.TempDir()}
	cmd := b.Build(toolReq(`{"path":"nope.txt"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
}

func TestWrite_NewFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	b := &WriteBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"newdir/file.go","content":"package foo\n"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "wrote")

	data, err := os.ReadFile(filepath.Join(root, "newdir", "file.go"))
	require.NoError(t, err)
	assert.Equal(t, "package foo\n", string(data))
}

func TestWrite_UndoRemovesCreatedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	b := &WriteBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"new.txt","content":"created"}`))
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)

	undo := cmd.Undo()

	require.Equal(t, core.ToolDone, undo.Signal)
	_, err := os.Stat(filepath.Join(root, "new.txt"))
	require.True(t, os.IsNotExist(err))
}

func TestWrite_Overwrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "exist.txt"), []byte("old"), 0o644))

	b := &WriteBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"exist.txt","content":"new"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolDone, res.Signal)

	data, err := os.ReadFile(filepath.Join(root, "exist.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))
}

func TestWrite_UndoRestoresOverwrittenFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "exist.txt"), []byte("old"), 0o600))

	b := &WriteBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"exist.txt","content":"new"}`))
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)

	undo := cmd.Undo()

	require.Equal(t, core.ToolDone, undo.Signal)
	data, err := os.ReadFile(filepath.Join(root, "exist.txt"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(data))
	info, err := os.Stat(filepath.Join(root, "exist.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWrite_UndoMementoDeclaresWorkspaceRestore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	b := &WriteBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"new.txt","content":"created"}`))
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)

	provider, ok := cmd.(core.UndoMementoProvider)
	require.True(t, ok)
	memento, err := provider.UndoMemento()

	require.NoError(t, err)
	require.Equal(t, core.UndoMementoReversible, memento.Kind)
	require.NoError(t, core.ValidateUndoMemento(memento))
	assert.Contains(t, string(memento.Payload), `"workspace_restore"`)
	assert.Contains(t, string(memento.Payload), `"new.txt"`)
}

func TestWrite_MissingParams(t *testing.T) {
	t.Parallel()

	b := &WriteBuilder{Root: t.TempDir()}
	cmd := b.Build(toolReq(`{"path":"f.txt"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "content")
}

func TestEdit_SingleMatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "e.txt"), []byte("hello world"), 0o644))

	b := &EditBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"e.txt","old_string":"hello","new_string":"goodbye"}`))
	res := cmd.Execute()
	assert.Equal(t, core.EditDone, res.Signal)
	assert.Contains(t, res.Output, "replacement applied")

	data, err := os.ReadFile(filepath.Join(root, "e.txt"))
	require.NoError(t, err)
	assert.Equal(t, "goodbye world", string(data))
}

func TestEdit_UndoRestoresOriginalFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "e.txt"), []byte("hello world"), 0o644))

	b := &EditBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"e.txt","old_string":"hello","new_string":"goodbye"}`))
	res := cmd.Execute()
	require.Equal(t, core.EditDone, res.Signal)

	undo := cmd.Undo()

	require.Equal(t, core.ToolDone, undo.Signal)
	data, err := os.ReadFile(filepath.Join(root, "e.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestEdit_NoMatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "e.txt"), []byte("hello"), 0o644))

	b := &EditBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"e.txt","old_string":"xyz","new_string":"abc"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "not found")
	assert.Contains(t, res.Output, "Current file contents:")
	assert.Contains(t, res.Output, "hello")
}

func TestEdit_NoMatchIncludesNumberedLines(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "e.txt"), []byte("line1\nline2\nline3"), 0o644))

	b := &EditBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"e.txt","old_string":"missing","new_string":"abc"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "1|line1")
	assert.Contains(t, res.Output, "2|line2")
	assert.Contains(t, res.Output, "3|line3")
}

func TestEdit_AmbiguousMatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "e.txt"), []byte("aaa"), 0o644))

	b := &EditBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"e.txt","old_string":"a","new_string":"b"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "ambiguous")
}

func TestListFiles_Basic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "top.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "deep.txt"), []byte("y"), 0o644))

	b := &ListFilesBuilder{Root: root}
	cmd := b.Build(toolReq(`{}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "top.txt")
	assert.Contains(t, res.Output, "sub/")
	assert.Contains(t, res.Output, "deep.txt")
}

func TestListFiles_SubPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a", "b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a", "b", "c.txt"), []byte("z"), 0o644))

	b := &ListFilesBuilder{Root: root}
	cmd := b.Build(toolReq(`{"path":"a"}`))
	res := cmd.Execute()
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "c.txt")
}

func TestIsBinary(t *testing.T) {
	t.Parallel()

	assert.True(t, IsBinary([]byte{0x00, 0x01}))
	assert.False(t, IsBinary([]byte("hello world")))
	assert.False(t, IsBinary(nil))
}
