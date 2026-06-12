// Copyright (c) 2026 Nokia. All rights reserved.

package core

import "context"

// StateStore persists JSON blobs for agent and command/domain state.
//
// The store deliberately does not track workspace files. Environment state is
// handled by Workspace so in-memory agent state and filesystem state can be
// checkpointed, restored, and reasoned about independently.
type StateStore interface {
	Save(ctx context.Context, key string, data []byte) error
	Load(ctx context.Context, key string) ([]byte, error)
	List(ctx context.Context, prefix string) ([]string, error)
	Delete(ctx context.Context, key string) error
}
