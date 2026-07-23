// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// These cover the value predicate word (srd041-value-predicate-tool,
// test-rel10.0-value-predicate). The word exists so a machine can branch on a
// value; the assertion that matters most is that a broken operand is a fault
// rather than a branch, because that failure is otherwise silent.

const (
	sigYes = core.Signal("Satisfied")
	sigNo  = core.Signal("Unsatisfied")
)

// run dispatches the predicate against a previous Result carrying prevOutput.
func run(t *testing.T, prevOutput string, b ValuePredicateBuilder) core.Result {
	t.Helper()
	b.ToolName = "predicate"
	b.Satisfied = sigYes
	b.Unsatisfied = sigNo
	return b.Build(core.Result{Output: prevOutput}).Execute()
}

// TestValuePredicateOperators proves every operator in the closed set decides
// correctly on both sides of its boundary. The equal case is what separates lt
// from lte and gt from gte, so each is exercised there rather than only well
// clear of it (srd041 AC2).
func TestValuePredicateOperators(t *testing.T) {
	for _, tc := range []struct {
		left, op, right string
		want            core.Signal
	}{
		{"5", OpEq, "5", sigYes},
		{"5", OpEq, "6", sigNo},
		{"5", OpNe, "6", sigYes},
		{"5", OpNe, "5", sigNo},
		{"5", OpLt, "5", sigNo},
		{"4", OpLt, "5", sigYes},
		{"5", OpLte, "5", sigYes},
		{"6", OpLte, "5", sigNo},
		{"5", OpGt, "5", sigNo},
		{"6", OpGt, "5", sigYes},
		{"5", OpGte, "5", sigYes},
		{"4", OpGte, "5", sigNo},
	} {
		t.Run(tc.left+" "+tc.op+" "+tc.right, func(t *testing.T) {
			got := run(t, "", ValuePredicateBuilder{Left: tc.left, Op: tc.op, Right: tc.right})
			if got.Signal != tc.want {
				t.Errorf("%s %s %s = %s, want %s (output %q)", tc.left, tc.op, tc.right, got.Signal, tc.want, got.Output)
			}
		})
	}
}

// TestValuePredicateEmptiness proves empty and non_empty judge the left operand
// alone. "0" and "false" are values rather than absences, which is the
// distinction R2.4 draws (srd041 AC2).
func TestValuePredicateEmptiness(t *testing.T) {
	for _, tc := range []struct {
		name, prev, selector string
		wantEmpty            bool
	}{
		{"empty string", `{"v":""}`, "$.v", true},
		{"empty list", `{"v":[]}`, "$.v", true},
		{"empty object", `{"v":{}}`, "$.v", true},
		{"null", `{"v":null}`, "$.v", true},
		{"zero string", `{"v":"0"}`, "$.v", false},
		{"false string", `{"v":"false"}`, "$.v", false},
		{"populated list", `{"v":[0]}`, "$.v", false},
		{"populated object", `{"v":{"a":1}}`, "$.v", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotEmpty := run(t, tc.prev, ValuePredicateBuilder{Left: tc.selector, Op: OpEmpty})
			wantEmpty := sigNo
			if tc.wantEmpty {
				wantEmpty = sigYes
			}
			if gotEmpty.Signal != wantEmpty {
				t.Errorf("empty(%s) = %s, want %s", tc.name, gotEmpty.Signal, wantEmpty)
			}
			// non_empty is the exact complement.
			gotNon := run(t, tc.prev, ValuePredicateBuilder{Left: tc.selector, Op: OpNonEmpty})
			if gotNon.Signal == gotEmpty.Signal {
				t.Errorf("non_empty(%s) = %s, same as empty; they must be complements", tc.name, gotNon.Signal)
			}
		})
	}

	// The emptiness operators ignore any right operand and do not need one.
	if err := ValidateValuePredicateConfig("predicate", "$.v", OpEmpty, "", "", "Yes", "No"); err != nil {
		t.Errorf("empty with no right operand should register: %v", err)
	}
}

// TestValuePredicateNumericCoercion is the case that motivated the default
// operand type. A REST read of a scalar body yields a string, so a count read
// back from a store arrives as "10"; comparing it as text puts it below "6"
// (srd041 AC3).
func TestValuePredicateNumericCoercion(t *testing.T) {
	const prev = `{"mapped":{"count":"10"}}`

	numeric := run(t, prev, ValuePredicateBuilder{Left: "$.mapped.count", Op: OpGt, Right: "6"})
	if numeric.Signal != sigYes {
		t.Errorf(`numeric "10" > "6" = %s, want %s; a string-typed count must compare as a number (output %q)`,
			numeric.Signal, sigYes, numeric.Output)
	}

	lexicographic := run(t, prev, ValuePredicateBuilder{
		Left: "$.mapped.count", Op: OpGt, Right: "6", OperandType: OperandString,
	})
	if lexicographic.Signal != sigNo {
		t.Errorf(`string "10" > "6" = %s, want %s; the string operand type orders lexicographically on purpose`,
			lexicographic.Signal, sigNo)
	}

	// A JSON number operand coerces the same way a string one does, so a
	// producer that emits a real number and one that emits text agree.
	asNumber := run(t, `{"mapped":{"count":10}}`, ValuePredicateBuilder{Left: "$.mapped.count", Op: OpGt, Right: "6"})
	if asNumber.Signal != sigYes {
		t.Errorf("numeric 10 > 6 = %s, want %s", asNumber.Signal, sigYes)
	}
}

// TestValuePredicateUnresolvedOperandIsAFault is the assertion that matters
// most. A mistyped or absent selector must not read as a legitimate negative: a
// machine would take the unsatisfied branch on a comparison that never ran, and
// nothing in the trace would say so (srd041 AC4).
func TestValuePredicateUnresolvedOperandIsAFault(t *testing.T) {
	for _, tc := range []struct {
		name       string
		prev       string
		builder    ValuePredicateBuilder
		wantInText string
	}{
		{
			name:       "left selector does not resolve",
			prev:       `{"mapped":{"count":"6"}}`,
			builder:    ValuePredicateBuilder{Left: "$.mapped.tally", Op: OpGt, Right: "0"},
			wantInText: "$.mapped.tally",
		},
		{
			name:       "right selector does not resolve",
			prev:       `{"mapped":{"count":"6"}}`,
			builder:    ValuePredicateBuilder{Left: "$.mapped.count", Op: OpGt, Right: "$.mapped.floor"},
			wantInText: "$.mapped.floor",
		},
		{
			name:       "left operand does not coerce",
			prev:       `{"v":"not-a-number"}`,
			builder:    ValuePredicateBuilder{Left: "$.v", Op: OpGt, Right: "0"},
			wantInText: "not-a-number",
		},
		{
			name:       "unary operand does not resolve",
			prev:       `{"v":"x"}`,
			builder:    ValuePredicateBuilder{Left: "$.missing", Op: OpNonEmpty},
			wantInText: "$.missing",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, tc.prev, tc.builder)
			if got.Signal != core.CommandError {
				t.Fatalf("signal = %s, want CommandError; a broken operand must not answer as a comparison", got.Signal)
			}
			if got.Signal == sigNo {
				t.Error("a fault was reported as the unsatisfied signal, which is the silent failure this word exists to avoid")
			}
			if got.Err == nil {
				t.Error("CommandError carries no error")
			}
			if !strings.Contains(got.Output, tc.wantInText) {
				t.Errorf("output %q does not name the offending operand %q", got.Output, tc.wantInText)
			}
		})
	}
}

// TestValuePredicateRegistrationRejectsMalformedConfig proves a misconfigured
// predicate fails before a run rather than at dispatch, with the tool name in
// the error (srd041 AC5, srd022 R3.3).
func TestValuePredicateRegistrationRejectsMalformedConfig(t *testing.T) {
	for _, tc := range []struct {
		name                                  string
		left, op, right, operandType, yes, no string
		wantInError                           string
	}{
		{"unknown operator", "$.v", "approximately", "1", "", "Yes", "No", "unknown operator"},
		{"missing satisfied signal", "$.v", OpGt, "1", "", "", "No", "satisfied signal"},
		{"missing unsatisfied signal", "$.v", OpGt, "1", "", "Yes", "", "unsatisfied signal"},
		{"missing left operand", "", OpGt, "1", "", "Yes", "No", "left operand"},
		{"binary operator with no right operand", "$.v", OpGt, "", "", "Yes", "No", "right operand"},
		{"unknown operand type", "$.v", OpGt, "1", "decimal", "Yes", "No", "operand type"},
		{"command-state operand", "$from(step).v", OpGt, "1", "", "Yes", "No", "command state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateValuePredicateConfig("countIsPositive", tc.left, tc.op, tc.right, tc.operandType, tc.yes, tc.no)
			if err == nil {
				t.Fatal("malformed config registered successfully")
			}
			if !strings.Contains(err.Error(), "countIsPositive") {
				t.Errorf("error %q does not name the tool", err)
			}
			if !strings.Contains(err.Error(), tc.wantInError) {
				t.Errorf("error %q does not name the offending field (%q)", err, tc.wantInError)
			}
		})
	}

	// The shipped shape registers.
	if err := ValidateValuePredicateConfig("countIsPositive", "$.mapped.count", OpGt, "0", "", "Ingested", "Rejected"); err != nil {
		t.Errorf("a well-formed predicate should register: %v", err)
	}
}

// TestValuePredicateDrivesTwoTransitions is the end-to-end shape: the same
// declared word reaches two different signals from the same config depending on
// the value, which is what makes it a branch in the grammar rather than a
// decision hidden in Go. It also pins the word's purity (srd041 AC1, AC6).
func TestValuePredicateDrivesTwoTransitions(t *testing.T) {
	predicate := ValuePredicateBuilder{Left: "$.mapped.count", Op: OpGt, Right: "0"}

	ingested := run(t, `{"mapped":{"count":"6"}}`, predicate)
	if ingested.Signal != sigYes {
		t.Errorf("a populated collection reached %s, want %s", ingested.Signal, sigYes)
	}
	empty := run(t, `{"mapped":{"count":"0"}}`, predicate)
	if empty.Signal != sigNo {
		t.Errorf("an empty collection reached %s, want %s", empty.Signal, sigNo)
	}
	if ingested.Signal == empty.Signal {
		t.Fatal("both values reached the same signal; the word is not branching")
	}

	// Output reports what was compared, so a trace shows the operands and not
	// only the branch taken (R4.4).
	if !strings.Contains(ingested.Output, "6") || !strings.Contains(ingested.Output, OpGt) {
		t.Errorf("output %q does not report the comparison", ingested.Output)
	}

	// Purity: the comparison records no side effect and reverses as a noop.
	cmd := predicate.Build(core.Result{Output: `{"mapped":{"count":"6"}}`})
	undo := cmd.Undo(ingested)
	if undo.Signal != core.NoopUndo("predicate").Signal {
		t.Errorf("undo signal = %s, want the noop undo signal", undo.Signal)
	}
}
