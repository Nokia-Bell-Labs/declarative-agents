// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"testing"
)

func TestGenerateLinearMachine_SinglePoint(t *testing.T) {
	gen := GenerateSpec{
		Name: "test-single",
		Points: []LoopSpec{
			{Steps: []string{"prepare", "run_agent", "check"}},
		},
	}

	spec := GenerateLinearMachine(gen)

	if spec.InitialState != "Point_0_Step_0" {
		t.Errorf("initial state = %q, want Point_0_Step_0", spec.InitialState)
	}

	// 3 step states + Done = 4
	if len(spec.States) != 4 {
		t.Errorf("states count = %d, want 4", len(spec.States))
	}

	// Seed + 3*(ToolDone+ToolFailed) = 7
	if len(spec.Transitions) != 7 {
		t.Errorf("transitions count = %d, want 7", len(spec.Transitions))
	}

	if err := validateSpec(spec); err != nil {
		t.Errorf("generated spec invalid: %v", err)
	}
}

func TestGenerateLinearMachine_MultiplePoints(t *testing.T) {
	gen := GenerateSpec{
		Name: "test-multi",
		Points: []LoopSpec{
			{Steps: []string{"prepare", "run"}, Vars: map[string]string{"model": "qwen"}},
			{Steps: []string{"prepare", "run"}, Vars: map[string]string{"model": "deepseek"}},
		},
	}

	spec := GenerateLinearMachine(gen)

	// 2 steps * 2 points + Done = 5
	if len(spec.States) != 5 {
		t.Errorf("states count = %d, want 5", len(spec.States))
	}

	if err := validateSpec(spec); err != nil {
		t.Errorf("generated spec invalid: %v", err)
	}

	// Check that Point_0_Step_1 ToolDone goes to Point_1_Step_0
	found := false
	for _, tr := range spec.Transitions {
		if tr.State == "Point_0_Step_1" && tr.Signal == "ToolDone" {
			if tr.Next != "Point_1_Step_0" {
				t.Errorf("Point_0_Step_1 ToolDone → %q, want Point_1_Step_0", tr.Next)
			}
			found = true
		}
	}
	if !found {
		t.Error("missing ToolDone transition from Point_0_Step_1")
	}
}

func TestGenerateLinearMachine_WithSummarize(t *testing.T) {
	gen := GenerateSpec{
		Name: "test-summarize",
		Points: []LoopSpec{
			{Steps: []string{"run"}},
		},
		DoneAction: "summarize",
	}

	spec := GenerateLinearMachine(gen)

	// 1 step + Summarize + Done = 3
	if len(spec.States) != 3 {
		t.Errorf("states count = %d, want 3", len(spec.States))
	}

	// Last step ToolDone should go to Summarize, not Done
	found := false
	for _, tr := range spec.Transitions {
		if tr.State == "Point_0_Step_0" && tr.Signal == "ToolDone" {
			if tr.Next != "Summarize" {
				t.Errorf("Point_0_Step_0 ToolDone → %q, want Summarize", tr.Next)
			}
			found = true
		}
	}
	if !found {
		t.Error("missing ToolDone transition from Point_0_Step_0")
	}

	if err := validateSpec(spec); err != nil {
		t.Errorf("generated spec invalid: %v", err)
	}
}

func TestGenerateLinearMachine_FailureSkipsToNext(t *testing.T) {
	gen := GenerateSpec{
		Name: "test-skip",
		Points: []LoopSpec{
			{Steps: []string{"prepare", "run"}},
			{Steps: []string{"prepare", "run"}},
		},
	}

	spec := GenerateLinearMachine(gen)

	// Point_0_Step_0 ToolFailed should skip to Point_1_Step_0
	for _, tr := range spec.Transitions {
		if tr.State == "Point_0_Step_0" && tr.Signal == "ToolFailed" {
			if tr.Next != "Point_1_Step_0" {
				t.Errorf("Point_0_Step_0 ToolFailed → %q, want Point_1_Step_0", tr.Next)
			}
			return
		}
	}
	t.Error("missing ToolFailed transition from Point_0_Step_0")
}

func TestGenerateLinearMachine_MarshalRoundtrip(t *testing.T) {
	gen := GenerateSpec{
		Name: "roundtrip",
		Points: []LoopSpec{
			{Steps: []string{"prep", "run", "check"}},
		},
		DoneAction: "summarize",
	}

	spec := GenerateLinearMachine(gen)
	data, err := MarshalMachineSpec(spec)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseMachineSpec(data)
	if err != nil {
		t.Fatalf("re-parse failed: %v\n%s", err, data)
	}

	if parsed.InitialState != spec.InitialState {
		t.Errorf("initial state mismatch: %q vs %q", parsed.InitialState, spec.InitialState)
	}
	if len(parsed.States) != len(spec.States) {
		t.Errorf("states count mismatch: %d vs %d", len(parsed.States), len(spec.States))
	}
}
