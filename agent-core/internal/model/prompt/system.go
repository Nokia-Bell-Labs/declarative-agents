// Copyright (c) 2026 Nokia. All rights reserved.

package prompt

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
)

//go:embed system.tmpl
var defaultSystemTemplate string

var systemTmpl = template.Must(template.New("system").Parse(defaultSystemTemplate))

// Envelope defines the open/close tags for wrapping tool calls.
// A nil *Envelope in PromptData means no wrapping (bare JSON).
type Envelope struct {
	Open  string
	Close string
}

// PromptData carries all values needed to render the system prompt
// template. Agents populate this from their Prompt struct, the tool
// manifest, and any model-specific envelope configuration.
type PromptData struct {
	Role         string
	Task         string
	Constraints  string
	OutputFormat string
	ToolManifest string
	Envelope     *Envelope
	StrictFormat bool
}

// RenderSystemPrompt executes the embedded system prompt template with
// the given data. On template execution error, it falls back to a
// simple concatenation of Role and Task.
func RenderSystemPrompt(data PromptData) string {
	var buf bytes.Buffer
	if err := systemTmpl.Execute(&buf, data); err != nil {
		return data.Role + "\n\n" + data.Task
	}
	return strings.TrimRight(buf.String(), "\n")
}

// RenderSystemPromptWith executes a custom template string instead of
// the embedded default. This allows agents to provide their own
// template layout while reusing PromptData.
func RenderSystemPromptWith(tmplText string, data PromptData) (string, error) {
	t, err := template.New("custom").Parse(tmplText)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
