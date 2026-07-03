// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// fileCheckpointKey is the single store key that holds the latest saved
// Position and Execution pair.
const fileCheckpointKey = "checkpoint/state"

// fileCheckpointDoc is the on-disk JSON shape: the resumable Position and the
// ordered Execution log saved as one unit (srd035-checkpoint-port R1.2).
type fileCheckpointDoc struct {
	Position  Position  `json:"position"`
	Execution Execution `json:"execution"`
}

// FileCheckpoint is a JSON-file adapter implementing the Checkpoint port. It is
// the interim persistent backend so cross-process suspend/resume keeps working
// until the Dolt adapter (srd036-dolt-state-persistence) becomes the default.
// It reuses FileStore for byte storage rather than reimplementing path handling.
type FileCheckpoint struct {
	store *FileStore
	ctx   context.Context
}

// NewFileCheckpoint returns a file-backed Checkpoint rooted at root.
func NewFileCheckpoint(root string) *FileCheckpoint {
	return &FileCheckpoint{store: NewFileStore(root), ctx: context.Background()}
}

func (f *FileCheckpoint) Save(position Position, execution Execution) error {
	data, err := json.Marshal(fileCheckpointDoc{Position: position, Execution: execution})
	if err != nil {
		return fmt.Errorf("file checkpoint: save: marshal: %w", err)
	}
	if err := f.store.Save(f.ctx, fileCheckpointKey, data); err != nil {
		return fmt.Errorf("file checkpoint: save: %w", err)
	}
	return nil
}

func (f *FileCheckpoint) Load() (Position, Execution, error) {
	data, err := f.store.Load(f.ctx, fileCheckpointKey)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Position{}, nil, ErrNoCheckpoint
		}
		return Position{}, nil, fmt.Errorf("file checkpoint: load: %w", err)
	}
	var doc fileCheckpointDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return Position{}, nil, fmt.Errorf("file checkpoint: load: decode: %w", err)
	}
	return doc.Position, doc.Execution, nil
}

var _ Checkpoint = (*FileCheckpoint)(nil)
