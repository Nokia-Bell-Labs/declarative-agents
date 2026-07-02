// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package materialize converts an ImplementationPlan into a beads issue
// in the target project via the bd CLI.
// Implements srd009-materializer.
package materialize

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/plan"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/subprocess"
)

const spanMaterialize = "materialize_task"

// MaterializeFailed indicates the materializer could not create a beads issue.
type MaterializeFailed struct {
	Cause  error
	Stderr string
}

func (e *MaterializeFailed) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("materialize_task failed: %v (stderr: %s)", e.Cause, e.Stderr)
	}
	return fmt.Sprintf("materialize_task failed: %v", e.Cause)
}

func (e *MaterializeFailed) Unwrap() error { return e.Cause }

// MaterializeTask creates beads issues from implementation plans.
type MaterializeTask struct {
	// RunBd executes the bd CLI. Tests inject a mock; production uses
	// NewMaterializeTask which wires defaultRunBd.
	RunBd func(ctx context.Context, args []string) ([]byte, error)
}

// NewMaterializeTask returns a MaterializeTask wired to the real bd CLI.
func NewMaterializeTask() *MaterializeTask {
	return &MaterializeTask{RunBd: defaultRunBd}
}

// Execute formats the plan as a beads issue description, invokes bd create
// in the given directory, and returns the created issue ID.
func (m *MaterializeTask) Execute(
	ctx context.Context,
	tracer tracing.Tracer,
	p plan.ImplementationPlan,
	dir string,
	taskDeps map[string]string,
) (string, error) {
	child, done := tracer.Push(spanMaterialize,
		attribute.String("task_id", p.Title),
		attribute.String("plan_title", p.Title),
		attribute.Int("dep_count", len(taskDeps)),
	)
	defer done()

	desc, err := FormatIssueDescription(p)
	if err != nil {
		mf := &MaterializeFailed{Cause: err}
		child.RecordError(mf)
		return "", mf
	}

	issueID, err := m.createIssue(ctx, desc, p.Title, dir, taskDeps)
	if err != nil {
		mf := &MaterializeFailed{Cause: err}
		child.RecordError(mf)
		return "", mf
	}

	child.SetAttributes(attribute.String("issue_id", issueID))
	return issueID, nil
}

// --- issue description formatting (R1) ---

type issueDescription struct {
	DeliverableType    string                 `yaml:"deliverable_type"`
	RequiredReading    []string               `yaml:"required_reading"`
	Files              []plan.PlanFile        `yaml:"files"`
	Requirements       []plan.PlanRequirement `yaml:"requirements"`
	DesignDecisions    []plan.PlanDecision    `yaml:"design_decisions,omitempty"`
	AcceptanceCriteria []plan.PlanCriterion   `yaml:"acceptance_criteria"`
}

// FormatIssueDescription produces a YAML string conforming to the
// issue-format constitution from an ImplementationPlan.
func FormatIssueDescription(p plan.ImplementationPlan) (string, error) {
	reading := make([]string, len(p.Files))
	for i, f := range p.Files {
		reading[i] = f.Path
	}

	desc := issueDescription{
		DeliverableType:    "code",
		RequiredReading:    reading,
		Files:              p.Files,
		Requirements:       p.Requirements,
		DesignDecisions:    p.DesignDecisions,
		AcceptanceCriteria: p.AcceptanceCriteria,
	}

	data, err := yaml.Marshal(desc)
	if err != nil {
		return "", fmt.Errorf("format issue description: %w", err)
	}
	return string(data), nil
}

// --- bd CLI invocation (R2, R3, R4, R5) ---

func (m *MaterializeTask) createIssue(
	ctx context.Context, desc, title, dir string, deps map[string]string,
) (string, error) {
	tmpFile, err := writeTempDesc(desc)
	if err != nil {
		return "", &MaterializeFailed{Cause: err}
	}
	defer os.Remove(tmpFile)

	args := buildBdArgs(tmpFile, title, dir, deps)

	out, err := m.RunBd(ctx, args)
	if err != nil {
		return "", &MaterializeFailed{Cause: err, Stderr: err.Error()}
	}

	return parseIssueID(out)
}

func writeTempDesc(desc string) (string, error) {
	f, err := os.CreateTemp("", "planner-desc-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp description: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(desc); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("write temp description: %w", err)
	}
	f.Close()
	return path, nil
}

func buildBdArgs(bodyFile, title, dir string, deps map[string]string) []string {
	args := []string{
		"create", "--title", title,
		"--body-file", bodyFile,
		"-C", dir,
		"--json",
		"-t", "task",
		"-l", "code",
	}
	if len(deps) > 0 {
		ids := make([]string, 0, len(deps))
		for _, id := range deps {
			ids = append(ids, id)
		}
		args = append(args, "--deps", strings.Join(ids, ","))
	}
	return args
}

func parseIssueID(data []byte) (string, error) {
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", &MaterializeFailed{Cause: fmt.Errorf("parse bd output: %w", err)}
	}
	if result.ID == "" {
		return "", &MaterializeFailed{Cause: fmt.Errorf("bd create returned empty issue ID")}
	}
	return result.ID, nil
}

func defaultRunBd(ctx context.Context, args []string) ([]byte, error) {
	out, err := subprocess.RunCLIOutput(ctx, "", "bd", args...)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}
