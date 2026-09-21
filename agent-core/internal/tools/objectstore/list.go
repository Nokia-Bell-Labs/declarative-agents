// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"gocloud.dev/blob"
)

type listCmd struct {
	opener     *Opener
	connection ConnectionConfig
	prefix     string
}

func (l *listCmd) Name() string                 { return "object_list" }
func (l *listCmd) Undo(core.Result) core.Result { return core.NoopUndo(l.Name()) }

func (l *listCmd) Execute() core.Result {
	ctx := context.Background()
	bucket, done, err := l.opener.Open(ctx, l.connection)
	if err != nil {
		return commandError(l.Name(), err)
	}
	defer done()
	iterator := bucket.List(&blob.ListOptions{Prefix: l.prefix})
	var keys []string
	for {
		object, err := iterator.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return commandError(l.Name(), fmt.Errorf("list prefix %q: %w", l.prefix, err))
		}
		keys = append(keys, object.Key)
	}
	output := strings.Join(keys, "\n")
	if len(keys) == 0 {
		output = fmt.Sprintf("no objects under prefix %q", l.prefix)
	}
	return core.Result{Output: output, Signal: core.ToolDone, CommandName: l.Name()}
}

// ListBuilder constructs object_list commands. An absent prefix lists the
// whole bucket, which is the natural reading of an empty prefix.
type ListBuilder struct {
	Opener     *Opener
	Connection ConnectionConfig
}

func (b *ListBuilder) Build(res core.Result) core.Command {
	prefix, _ := extractStringParam(res.Output, "prefix")
	return &listCmd{opener: b.Opener, connection: b.Connection, prefix: prefix}
}
