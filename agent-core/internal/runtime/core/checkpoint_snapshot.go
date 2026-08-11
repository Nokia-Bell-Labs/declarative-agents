// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
)

func (r *loopRunner) foldCheckpointSnapshots(pos *Position) error {
	if err := r.foldSnapshot(
		"conversation", r.params.Hooks.SnapshotConversation,
		&pos.Snapshot.Conversation, ErrConversationSnapshotFailed,
	); err != nil {
		return err
	}
	return r.foldSnapshot(
		"domain", r.params.Hooks.SnapshotDomain,
		&pos.Snapshot.Domain, ErrDomainSnapshotFailed,
	)
}

func (r *loopRunner) foldSnapshot(
	name string,
	snapshot func() (json.RawMessage, error),
	target *json.RawMessage,
	classification error,
) error {
	if snapshot == nil {
		return nil
	}
	value, err := snapshot()
	if err != nil {
		r.trace.Event("checkpoint."+name+"_snapshot_failed",
			attribute.Int("iteration", r.iteration),
			attribute.String("error", err.Error()),
		)
		return fmt.Errorf("%w at iteration %d: %w", classification, r.iteration, err)
	}
	*target = value
	return nil
}
