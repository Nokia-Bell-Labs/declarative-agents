// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package plan

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed prompt.tmpl
var promptTemplate string

// TaskContext holds the task-level information for prompt assembly.
type TaskContext struct {
	ID    string
	SRDID string
	Items []TaskItem
}

// TaskItem is one requirement item from the task.
type TaskItem struct {
	ID   string
	Text string
}

// SRDContext holds the SRD metadata needed for prompt assembly.
type SRDContext struct {
	Problem            string
	Goals              []string
	AcceptanceCriteria []string
}

// DepItem describes an already-implemented requirement with its output files.
type DepItem struct {
	ID    string
	Files []string
}

type promptData struct {
	Task       TaskContext
	SRD        SRDContext
	DepContext  []DepItem
	FailureCtx []string
}

var tmpl = template.Must(template.New("prompt").Funcs(template.FuncMap{
	"join": strings.Join,
}).Parse(promptTemplate))

// AssemblePrompt builds a planning prompt from task context, SRD metadata,
// dependency context, and optional failure context for retries.
// Implements srd008-planning-engine R1.
func AssemblePrompt(task TaskContext, srd SRDContext, depContext []DepItem, failureCtx []string) (string, error) {
	data := promptData{
		Task:       task,
		SRD:        srd,
		DepContext:  depContext,
		FailureCtx: failureCtx,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("assemble prompt: %w", err)
	}
	return buf.String(), nil
}
