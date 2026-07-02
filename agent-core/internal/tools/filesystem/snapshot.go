// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

type fileSnapshot struct {
	Path    string
	Exists  bool
	Content []byte
	Mode    os.FileMode
}

type workspaceUndoPayload struct {
	WorkspaceRestore struct {
		Paths []string `json:"paths"`
	} `json:"workspace_restore"`
}

func snapshotFile(root, resolved string) (fileSnapshot, error) {
	snap := fileSnapshot{Path: RelPath(root, resolved)}
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return snap, nil
		}
		return fileSnapshot{}, err
	}
	if info.IsDir() {
		return fileSnapshot{}, fmt.Errorf("%s is a directory", snap.Path)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return fileSnapshot{}, err
	}
	snap.Exists = true
	snap.Content = append([]byte(nil), data...)
	snap.Mode = info.Mode().Perm()
	return snap, nil
}

func undoFileSnapshot(commandName, root string, snap fileSnapshot, ok bool) core.Result {
	if !ok {
		err := fmt.Errorf("undo %s: no file snapshot recorded", commandName)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	resolved, err := ValidatePath(root, snap.Path)
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
		}
		resolved = filepath.Join(root, filepath.FromSlash(snap.Path))
	}
	return restoreSnapshot(commandName, resolved, snap)
}

func restoreSnapshot(commandName, resolved string, snap fileSnapshot) core.Result {
	if !snap.Exists {
		if err := os.Remove(resolved); err != nil && !os.IsNotExist(err) {
			return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
		}
		return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: fmt.Sprintf("undo: removed created file %s", snap.Path)}
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	if err := os.WriteFile(resolved, snap.Content, snap.Mode); err != nil {
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: fmt.Sprintf("undo: restored %s", snap.Path)}
}

func fileWorkspaceMemento(commandName string, snap fileSnapshot, ok bool) (core.UndoMemento, error) {
	if !ok {
		return core.UndoMemento{}, fmt.Errorf("%w: no file snapshot recorded for %s", core.ErrUndoMementoMissing, commandName)
	}
	payload := workspaceUndoPayload{}
	payload.WorkspaceRestore.Paths = []string{snap.Path}
	return core.NewUndoMemento(commandName, core.UndoMementoReversible, payload)
}
