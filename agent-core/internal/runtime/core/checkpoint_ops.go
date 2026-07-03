// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ResolveLatestCheckpointID scans the store for all checkpoint/ keys and
// returns the ID of the most recent one by timestamp. If requested is a
// non-empty string other than "latest", it is returned as-is.
func ResolveLatestCheckpointID(ctx context.Context, store StateStore, requested string) (string, error) {
	if requested != "" && requested != "latest" {
		return requested, nil
	}
	keys, err := store.List(ctx, "checkpoint/")
	if err != nil {
		return "", fmt.Errorf("list checkpoints: %w", err)
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no checkpoints found")
	}
	sort.Strings(keys)
	var latest CheckpointRecord
	var latestID string
	for _, key := range keys {
		id := strings.TrimPrefix(key, "checkpoint/")
		cp, err := LoadCheckpoint(ctx, store, id)
		if err != nil {
			continue
		}
		if latestID == "" || cp.Timestamp.After(latest.Timestamp) || (cp.Timestamp.Equal(latest.Timestamp) && id > latestID) {
			latest = cp
			latestID = id
		}
	}
	if latestID == "" {
		return "", fmt.Errorf("no readable checkpoints found")
	}
	return latestID, nil
}

// FormatCheckpointHistory renders a checkpoint as a human-readable digest
// showing the checkpoint metadata, current state, and history entries.
func FormatCheckpointHistory(cp CheckpointRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "checkpoint: %s\n", cp.ID)
	fmt.Fprintf(&b, "iteration: %d\n", cp.Iteration)
	fmt.Fprintf(&b, "state: %s\n", cp.AgentState.State)
	if cp.WorkspaceRef != "" {
		fmt.Fprintf(&b, "workspace_ref: %s\n", cp.WorkspaceRef)
	}
	if len(cp.History) == 0 {
		b.WriteString("history: <empty>\n")
		return b.String()
	}
	b.WriteString("history:\n")
	for _, entry := range cp.History {
		fmt.Fprintf(&b, "  %d  %s  %s -> %s  signal=%s", entry.Iteration, entry.CommandName, entry.FromState, entry.ToState, entry.Signal)
		if entry.WorkspaceRef != "" {
			fmt.Fprintf(&b, "  workspace=%s", entry.WorkspaceRef)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
