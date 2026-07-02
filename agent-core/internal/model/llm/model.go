// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import "time"

// ModelInfo describes a model available from an LLM backend.
// Common fields are top-level; provider-specific metadata lives in
// the Details map (e.g. "parameter_size", "quantization_level", "family").
type ModelInfo struct {
	Name       string            `json:"name"`
	Size       int64             `json:"size"`
	ModifiedAt time.Time         `json:"modified_at"`
	Provider   string            `json:"provider"`
	Details    map[string]string `json:"details,omitempty"`
}
