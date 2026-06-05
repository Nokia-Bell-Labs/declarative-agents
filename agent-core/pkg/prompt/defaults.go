// Copyright (c) 2026 Nokia. All rights reserved.

// Compiled default strings for Prompt fields.
// These are baked into the binary and not loaded from disk at runtime.
// Agents should override them with domain-specific values.
package prompt

// DefaultRole describes a general-purpose coding assistant persona.
// Agents should replace this with their own role description.
const DefaultRole = `You are a coding assistant that reads, writes, and edits files ` +
	`in a workspace using the provided tools.`

// DefaultConstraints provides baseline behavioral rules. Agents may
// extend or replace these with domain-specific constraints.
const DefaultConstraints = `Respond only with a single JSON tool call per turn. ` +
	`Never produce plain-text answers. ` +
	`Stay within the workspace directory.`

// DefaultOutputFormat describes the expected JSON response structure.
const DefaultOutputFormat = `Return a JSON object with two fields: ` +
	`"tool" (the tool name) and "params" (an object whose keys match the tool's parameter schema).`
