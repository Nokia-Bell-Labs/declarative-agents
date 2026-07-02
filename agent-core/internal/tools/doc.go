// Copyright (c) 2026 Nokia. All rights reserved.

// Package tools groups the private tool packages used by profile-driven agents.
//
// Focused child packages own the long-term contracts and implementations:
// catalog loads and selects declarations, registry wires builders, exec runs
// YAML-defined commands, filesystem owns workspace file tools, llm owns model
// boundary tools, control owns loop-control boundaries, lifecycle owns suspend
// and checkpoint tools, validation owns audit tools, and undo owns shared
// compensation payloads. The stl child package remains only as a compatibility
// facade for callers that have not migrated.
package tools
