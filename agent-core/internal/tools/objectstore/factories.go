// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitObjectRead  = "object_read"
	InitObjectWrite = "object_write"
	InitObjectList  = "object_list"
)

// FactoryDeps carries the process-shared opener, so every word in a run sees
// the same mem:// buckets and the same driver table.
type FactoryDeps struct {
	Opener *Opener
}

// RegisterFactories registers the three objectstore inits. The connection is
// resolved at configuration time, so a bad declaration fails the load rather
// than the first call (srd059 R2.1).
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	opener := deps.Opener
	if opener == nil {
		opener = NewOpener()
	}
	register := func(init string, build func(ConnectionConfig) core.Builder) {
		br.Register(init, func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
			connection, err := DecodeConfig(def)
			if err != nil {
				return nil, err
			}
			if _, _, err := probeScheme(connection); err != nil {
				return nil, err
			}
			return build(connection), nil
		})
	}
	register(InitObjectRead, func(c ConnectionConfig) core.Builder { return &ReadBuilder{Opener: opener, Connection: c} })
	register(InitObjectWrite, func(c ConnectionConfig) core.Builder { return &WriteBuilder{Opener: opener, Connection: c} })
	register(InitObjectList, func(c ConnectionConfig) core.Builder { return &ListBuilder{Opener: opener, Connection: c} })
}
