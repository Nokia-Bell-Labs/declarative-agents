// Copyright (c) 2026 Nokia. All rights reserved.

package core

import "context"

// StateWriter persists JSON blobs for agent and command/domain state.
type StateWriter interface {
	Save(ctx context.Context, key string, data []byte) error
}

// StateReader loads JSON blobs and lists persisted keys.
type StateReader interface {
	Load(ctx context.Context, key string) ([]byte, error)
	List(ctx context.Context, prefix string) ([]string, error)
}

// StateDeleter removes persisted state entries.
type StateDeleter interface {
	Delete(ctx context.Context, key string) error
}

// StateStore composes narrow state persistence ports.
//
// The store deliberately does not track workspace files. Environment state is
// handled by Workspace so in-memory agent state and filesystem state can be
// checkpointed, restored, and reasoned about independently.
type StateStore interface {
	StateWriter
	StateReader
	StateDeleter
}
