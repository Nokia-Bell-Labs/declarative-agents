// Copyright (c) 2026 Nokia. All rights reserved.

// Package stl is a temporary compatibility facade for the split tool packages.
//
// New code should import the focused package that owns the contract or
// implementation:
//   - catalog for ToolDef, declaration loading, selection, and config decoding
//   - registry for builtin factory registration and dynamic tool dispatch
//   - exec for YAML-defined exec builders, subprocess helpers, and metrics
//   - filesystem for read, write, edit, find, list_files, and path helpers
//   - llm for invoke_llm, parse_response, parse-error policy, and history tools
//   - control for child-agent and loop-control boundary tools
//   - lifecycle for suspend, checkpoint_history, and checkpoint_rollback
//   - validation for validate and specification validation tools
//   - undo for shared undo memento payloads and boundary compensation
//
// This package keeps aliases and small wrapper functions for callers that have
// not migrated yet. Do not add new long-term tool implementations here.
package stl
