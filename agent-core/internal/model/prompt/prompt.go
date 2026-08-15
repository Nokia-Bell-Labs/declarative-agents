// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package prompt defines structured prompt data rendered by model adapters.
package prompt

// Prompt carries the four sections of a system-role message.
type Prompt struct {
	Role         string
	Task         string
	Constraints  string
	OutputFormat string
}
