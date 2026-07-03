// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type fileSnapshot struct {
	Path    string
	Exists  bool
	Content []byte
	Mode    os.FileMode
}

// fileReceipt is the opaque, tool-owned rollback context write and edit encode
// into Result.Receipt during Execute. It carries enough to restore the file
// after a process restart, so a fresh command instance can Undo from the
// persisted receipt alone (srd035-checkpoint-port R3; #44 R2). Only the
// filesystem tools decode it; the engine and adapters treat it as opaque.
type fileReceipt struct {
	Path    string      `json:"path"`
	Existed bool        `json:"existed"`
	Content []byte      `json:"content,omitempty"`
	Mode    os.FileMode `json:"mode,omitempty"`
}

// encodeFileReceipt serializes the prior-state snapshot into an opaque receipt.
func encodeFileReceipt(snap fileSnapshot) string {
	b, err := json.Marshal(fileReceipt{
		Path:    snap.Path,
		Existed: snap.Exists,
		Content: snap.Content,
		Mode:    snap.Mode,
	})
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeFileReceipt reconstructs the prior-state snapshot from an opaque receipt.
// It reports ok=false for an empty receipt so callers can fall back to an
// in-memory snapshot on the live undo path.
func decodeFileReceipt(receipt string) (fileSnapshot, bool, error) {
	if receipt == "" {
		return fileSnapshot{}, false, nil
	}
	var r fileReceipt
	if err := json.Unmarshal([]byte(receipt), &r); err != nil {
		return fileSnapshot{}, false, err
	}
	return fileSnapshot{Path: r.Path, Exists: r.Existed, Content: r.Content, Mode: r.Mode}, true, nil
}

// undoFileFromReceipt reverses a write or edit. It prefers the tool-owned
// receipt carried on the prior Result (works from a fresh command instance) and
// falls back to the in-memory snapshot for the live in-process undo path.
func undoFileFromReceipt(commandName, root, receipt string, snap fileSnapshot, ok bool) core.Result {
	if decoded, present, err := decodeFileReceipt(receipt); err != nil {
		e := fmt.Errorf("undo %s: decode receipt: %w", commandName, err)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: e.Error(), Err: e}
	} else if present {
		snap, ok = decoded, true
	}
	return undoFileSnapshot(commandName, root, snap, ok)
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
