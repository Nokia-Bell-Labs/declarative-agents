// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
)

// UndoStrategy names the write's registered rollback strategy: the receipt
// snapshot restores the prior object or deletes the created one
// (srd059 R3.2).
const UndoStrategy = "object_snapshot_restore"

// objectReceipt is what a write leaves behind: enough to put the world back
// with no further input (core.Reverser; srd035-checkpoint-port R3).
type objectReceipt struct {
	Key     string `json:"key"`
	Existed bool   `json:"existed"`
	// PriorBase64 carries the prior object bytes when Existed; base64 keeps
	// the receipt a printable JSON string whatever the object held.
	PriorBase64 string `json:"prior_base64,omitempty"`
}

func encodeObjectReceipt(receipt objectReceipt) string {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func decodeObjectReceipt(encoded string) (objectReceipt, error) {
	var receipt objectReceipt
	if err := json.Unmarshal([]byte(encoded), &receipt); err != nil {
		return objectReceipt{}, fmt.Errorf("decode object receipt: %w", err)
	}
	if receipt.Key == "" {
		return objectReceipt{}, fmt.Errorf("object receipt names no key")
	}
	return receipt, nil
}

type writeCmd struct {
	opener     *Opener
	connection ConnectionConfig
	key        string
	content    string
}

func (w *writeCmd) Name() string { return "object_write" }

func (w *writeCmd) Execute() core.Result {
	ctx := context.Background()
	bucket, done, err := w.opener.Open(ctx, w.connection)
	if err != nil {
		return commandError(w.Name(), err)
	}
	defer done()
	receipt := objectReceipt{Key: w.key}
	prior, err := bucket.ReadAll(ctx, w.key)
	switch {
	case err == nil:
		receipt.Existed = true
		receipt.PriorBase64 = base64.StdEncoding.EncodeToString(prior)
	case gcerrors.Code(err) == gcerrors.NotFound:
	default:
		return commandError(w.Name(), fmt.Errorf("snapshot prior object %q: %w", w.key, err))
	}
	if err := bucket.WriteAll(ctx, w.key, []byte(w.content), nil); err != nil {
		return commandError(w.Name(), fmt.Errorf("write object %q: %w", w.key, err))
	}
	return core.Result{
		Output:      fmt.Sprintf("wrote %d bytes to %s", len(w.content), w.key),
		Signal:      core.ToolDone,
		CommandName: w.Name(),
		Receipt:     encodeObjectReceipt(receipt),
	}
}

// Undo walks the receipt alone: restore the prior bytes, or delete the
// object the write created (srd059 R3.2).
func (w *writeCmd) Undo(prior core.Result) core.Result {
	receipt, err := decodeObjectReceipt(prior.Receipt)
	if err != nil {
		return commandError(w.Name(), err)
	}
	ctx := context.Background()
	bucket, done, err := w.opener.Open(ctx, w.connection)
	if err != nil {
		return commandError(w.Name(), err)
	}
	defer done()
	if receipt.Existed {
		bytes, err := base64.StdEncoding.DecodeString(receipt.PriorBase64)
		if err != nil {
			return commandError(w.Name(), fmt.Errorf("decode prior object for %q: %w", receipt.Key, err))
		}
		if err := bucket.WriteAll(ctx, receipt.Key, bytes, nil); err != nil {
			return commandError(w.Name(), fmt.Errorf("restore object %q: %w", receipt.Key, err))
		}
		return core.Result{
			Output:      fmt.Sprintf("restored prior object %s", receipt.Key),
			Signal:      core.ToolDone,
			CommandName: w.Name(),
		}
	}
	if err := deleteIgnoringAbsence(ctx, bucket, receipt.Key); err != nil {
		return commandError(w.Name(), fmt.Errorf("delete created object %q: %w", receipt.Key, err))
	}
	return core.Result{
		Output:      fmt.Sprintf("deleted created object %s", receipt.Key),
		Signal:      core.ToolDone,
		CommandName: w.Name(),
	}
}

// deleteIgnoringAbsence makes undo idempotent: undoing a create twice finds
// the object already gone, which is the state undo wants.
func deleteIgnoringAbsence(ctx context.Context, bucket *blob.Bucket, key string) error {
	err := bucket.Delete(ctx, key)
	if err != nil && gcerrors.Code(err) != gcerrors.NotFound {
		return err
	}
	return nil
}

// WriteBuilder constructs object_write commands and their receipt-driven
// reversers.
type WriteBuilder struct {
	Opener     *Opener
	Connection ConnectionConfig
}

func (b *WriteBuilder) Build(res core.Result) core.Command {
	key, ok := extractStringParam(res.Output, "key")
	if !ok || key == "" {
		return missingParam("object_write", "key")
	}
	content, ok := extractStringParam(res.Output, "content")
	if !ok {
		return missingParam("object_write", "content")
	}
	return &writeCmd{opener: b.Opener, connection: b.Connection, key: key, content: content}
}

// BuildReverser returns a write command configured only for receipt-driven
// Undo: the receipt carries the prior object state, so the rollback receipt
// walk needs no key or content input (core.Reverser).
func (b *WriteBuilder) BuildReverser() core.Command {
	return &writeCmd{opener: b.Opener, connection: b.Connection}
}
