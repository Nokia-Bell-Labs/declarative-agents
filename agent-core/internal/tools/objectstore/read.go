// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"context"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type readCmd struct {
	opener     *Opener
	connection ConnectionConfig
	key        string
}

func (r *readCmd) Name() string                 { return "object_read" }
func (r *readCmd) Undo(core.Result) core.Result { return core.NoopUndo(r.Name()) }

func (r *readCmd) Execute() core.Result {
	ctx := context.Background()
	bucket, done, err := r.opener.Open(ctx, r.connection)
	if err != nil {
		return commandError(r.Name(), err)
	}
	defer done()
	content, err := bucket.ReadAll(ctx, r.key)
	if err != nil {
		return commandError(r.Name(), fmt.Errorf("read object %q: %w", r.key, err))
	}
	return core.Result{Output: string(content), Signal: core.ToolDone, CommandName: r.Name()}
}

// ReadBuilder constructs object_read commands.
type ReadBuilder struct {
	Opener     *Opener
	Connection ConnectionConfig
}

func (b *ReadBuilder) Build(res core.Result) core.Command {
	key, ok := extractStringParam(res.Output, "key")
	if !ok || key == "" {
		return missingParam("object_read", "key")
	}
	return &readCmd{opener: b.Opener, connection: b.Connection, key: key}
}
