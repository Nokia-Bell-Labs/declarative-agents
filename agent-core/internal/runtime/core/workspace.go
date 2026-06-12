// Copyright (c) 2026 Nokia. All rights reserved.

package core

import "context"

// Workspace tracks the external environment state for rollback support.
//
// A Workspace represents the filesystem layer, not agent or command state.
// Implementations may use git commits, snapshots, overlays, or another
// mechanism, but callers only rely on opaque refs returned by Checkpoint.
type Workspace interface {
	Checkpoint(ctx context.Context, label string) (ref string, err error)
	Restore(ctx context.Context, ref string) error
	CurrentRef(ctx context.Context) (string, error)
}
