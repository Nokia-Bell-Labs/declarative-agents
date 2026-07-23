// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// The value predicate word (srd041-value-predicate-tool). A machine branches on
// signals and signals come from words, so branching on a value needs a word that
// emits one from a comparison. This is that word: it resolves two operands
// against the previous Result, compares them under a declared operator, and
// emits one of two machine-declared signals.
//
// The failure separation in Execute is the point of the word rather than a
// detail of it (R4.2): an operand that will not resolve or coerce emits
// CommandError, never the unsatisfied signal. A machine that read a broken
// selector as a legitimate negative would take a plausible-looking branch on a
// comparison that never ran, and nothing in the trace would say so.

// Comparison operators (srd041 R2.1). The set is closed: extending it amends
// R2.5 rather than adding an escape hatch, so what a machine may branch on stays
// enumerable from the specification.
const (
	OpEq       = "eq"
	OpNe       = "ne"
	OpLt       = "lt"
	OpLte      = "lte"
	OpGt       = "gt"
	OpGte      = "gte"
	OpEmpty    = "empty"
	OpNonEmpty = "non_empty"
)

// Operand types (srd041 R3.1). number is the default because the ordering
// operators are the reason to reach for this word.
const (
	OperandNumber = "number"
	OperandString = "string"
)

// Operand prefixes (srd041 R1.3). selectorPrefix addresses the previous Result;
// fromPrefix addresses an earlier labelled step through the command-state view,
// which is how a predicate compares across a word whose Result carries nothing
// forward -- an exec word, for instance (GH-774). Anything else is a literal.
const (
	selectorPrefix = "$."
	fromPrefix     = "$from("
)

// unaryOps test the left operand alone and ignore any right operand (R2.3).
var unaryOps = map[string]bool{OpEmpty: true, OpNonEmpty: true}

var knownOps = map[string]bool{
	OpEq: true, OpNe: true, OpLt: true, OpLte: true, OpGt: true, OpGte: true,
	OpEmpty: true, OpNonEmpty: true,
}

// ValuePredicateBuilder constructs value predicate commands from declared config.
type ValuePredicateBuilder struct {
	ToolName    string
	Left        string
	Op          string
	Right       string
	OperandType string
	Satisfied   core.Signal
	Unsatisfied core.Signal
}

// ValidateValuePredicateConfig checks a declared predicate before it can reach a
// run (srd041 R1.6). A misconfigured predicate fails registration rather than
// dispatch, because a machine that reaches an unknown operator mid-run has
// already committed to a branch it cannot take.
func ValidateValuePredicateConfig(toolName, left, op, right, operandType, satisfied, unsatisfied string) error {
	if !knownOps[op] {
		return fmt.Errorf("tool %q: unknown operator %q; the operator set is closed (srd041 R2.1)", toolName, op)
	}
	if left == "" {
		return fmt.Errorf("tool %q: config names no left operand (srd041 R1.2)", toolName)
	}
	if satisfied == "" {
		return fmt.Errorf("tool %q: config names no satisfied signal (srd041 R1.4)", toolName)
	}
	if unsatisfied == "" {
		return fmt.Errorf("tool %q: config names no unsatisfied signal (srd041 R1.4)", toolName)
	}
	if !unaryOps[op] && right == "" {
		return fmt.Errorf("tool %q: operator %q needs a right operand (srd041 R1.2)", toolName, op)
	}
	switch operandType {
	case "", OperandNumber, OperandString:
	default:
		return fmt.Errorf("tool %q: unknown operand type %q, want %q or %q (srd041 R3.1)",
			toolName, operandType, OperandNumber, OperandString)
	}
	// A $from(label).path operand is validated here so a malformed one fails
	// registration rather than at dispatch (R1.3, R1.6). ParseFromSelector
	// rejects empty labels and paths and malformed parentheses (srd038 R2.5).
	for _, operand := range []string{left, right} {
		if !strings.HasPrefix(operand, fromPrefix) {
			continue
		}
		if _, _, ok := core.ParseFromSelector(operand); !ok {
			return fmt.Errorf("tool %q: operand %q is not a valid $from(label).path selector (srd041 R1.3)",
				toolName, operand)
		}
	}
	return nil
}

func (b ValuePredicateBuilder) Build(res core.Result) core.Command {
	operandType := b.OperandType
	if operandType == "" {
		operandType = OperandNumber
	}
	return &valuePredicateCmd{
		name:        b.ToolName,
		left:        b.Left,
		op:          b.Op,
		right:       b.Right,
		operandType: operandType,
		satisfied:   b.Satisfied,
		unsatisfied: b.Unsatisfied,
		prev:        res,
	}
}

type valuePredicateCmd struct {
	name        string
	left        string
	op          string
	right       string
	operandType string
	satisfied   core.Signal
	unsatisfied core.Signal
	prev        core.Result
	view        core.CommandStateView
}

func (c *valuePredicateCmd) Name() string { return c.name }

// SetCommandState receives the read-only command-state view, so a $from(label)
// operand can address an earlier step. The engine injects it before dispatch.
func (c *valuePredicateCmd) SetCommandState(view core.CommandStateView) { c.view = view }

var _ core.CommandStateAware = (*valuePredicateCmd)(nil)

// Undo is a noop: comparing two values changes nothing (srd041 R5.2).
func (c *valuePredicateCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c *valuePredicateCmd) Execute() core.Result {
	output := decodePreviousOutput(c.prev.Output)

	left, err := c.resolveOperand(c.left, output)
	if err != nil {
		return c.fault(err)
	}
	if unaryOps[c.op] {
		return c.signalFor(isEmpty(left) == (c.op == OpEmpty), left, nil)
	}

	right, err := c.resolveOperand(c.right, output)
	if err != nil {
		return c.fault(err)
	}
	held, err := c.compare(left, right)
	if err != nil {
		return c.fault(err)
	}
	return c.signalFor(held, left, right)
}

// resolveOperand returns a literal unchanged, resolves a $. selector against the
// previous Result, and resolves a $from(label).path selector against the
// command-state view. Either selector form that does not resolve is an error
// rather than a zero value, so it reaches Execute's fault path instead of
// silently comparing as empty (srd041 R4.2) -- a label that never ran must not
// read as a false comparison any more than a mistyped path does.
func (c *valuePredicateCmd) resolveOperand(operand string, output map[string]interface{}) (interface{}, error) {
	if strings.HasPrefix(operand, fromPrefix) {
		value, err := core.ResolveFromSelector(c.view, operand)
		if err != nil {
			return nil, err
		}
		return value, nil
	}
	if !strings.HasPrefix(operand, selectorPrefix) {
		return operand, nil
	}
	parsed, ok := core.ParseSelector(operand)
	if !ok {
		return nil, fmt.Errorf("operand %q is not a valid selector", operand)
	}
	value, ok := parsed.Resolve(output)
	if !ok {
		return nil, fmt.Errorf("operand %q did not resolve against the previous result", operand)
	}
	return value, nil
}

// compare applies the operator under the declared operand type. Coercion is
// declared rather than inferred from the values (srd041 R3.5): a predicate that
// compared numerically for one response and lexicographically for the next would
// not be a contract.
func (c *valuePredicateCmd) compare(left, right interface{}) (bool, error) {
	if c.operandType == OperandString {
		return compareStrings(c.op, formatOperand(left), formatOperand(right)), nil
	}
	leftNum, err := coerceNumber(left)
	if err != nil {
		return false, fmt.Errorf("left operand: %w", err)
	}
	rightNum, err := coerceNumber(right)
	if err != nil {
		return false, fmt.Errorf("right operand: %w", err)
	}
	return compareNumbers(c.op, leftNum, rightNum), nil
}

func compareNumbers(op string, left, right float64) bool {
	switch op {
	case OpEq:
		return left == right
	case OpNe:
		return left != right
	case OpLt:
		return left < right
	case OpLte:
		return left <= right
	case OpGt:
		return left > right
	case OpGte:
		return left >= right
	}
	return false
}

func compareStrings(op string, left, right string) bool {
	switch op {
	case OpEq:
		return left == right
	case OpNe:
		return left != right
	case OpLt:
		return left < right
	case OpLte:
		return left <= right
	case OpGt:
		return left > right
	case OpGte:
		return left >= right
	}
	return false
}

// coerceNumber converts a resolved operand to a number. A REST read of a scalar
// body yields a string, so a count read back from a store arrives as "6"; under
// the default operand type it must compare as six rather than as text, or "10"
// would order below it (srd041 R3.2).
func coerceNumber(value interface{}) (float64, error) {
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case json.Number:
		number, err := typed.Float64()
		if err != nil {
			return 0, fmt.Errorf("%q is not a number", typed.String())
		}
		return number, nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, fmt.Errorf("%q is not a number", typed)
		}
		return number, nil
	case bool:
		return 0, fmt.Errorf("%t is not a number", typed)
	case nil:
		return 0, fmt.Errorf("null is not a number")
	default:
		return 0, fmt.Errorf("%v is not a number", typed)
	}
}

// isEmpty reports the emptiness R2.4 defines: a zero-length string, an empty
// list, an empty object, or a null. Every other resolved value is non-empty,
// including "0" and "false", which are values rather than absences.
func isEmpty(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []interface{}:
		return len(typed) == 0
	case map[string]interface{}:
		return len(typed) == 0
	default:
		return false
	}
}

func formatOperand(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// signalFor emits the declared outcome signal and reports what was compared, so
// a run's trace shows the operands and not only the branch taken (srd041 R4.4).
func (c *valuePredicateCmd) signalFor(held bool, left, right interface{}) core.Result {
	signal := c.unsatisfied
	if held {
		signal = c.satisfied
	}
	var output string
	if unaryOps[c.op] {
		output = fmt.Sprintf("%s(%s) = %t", c.op, formatOperand(left), held)
	} else {
		output = fmt.Sprintf("%s %s %s = %t (%s)",
			formatOperand(left), c.op, formatOperand(right), held, c.operandType)
	}
	return core.Result{Signal: signal, CommandName: c.Name(), Output: output}
}

// fault emits CommandError for an operand that would not resolve or coerce. It
// is deliberately not the unsatisfied signal (srd041 R4.2).
func (c *valuePredicateCmd) fault(err error) core.Result {
	wrapped := fmt.Errorf("%s: %w", c.Name(), err)
	return core.Result{
		Signal:      core.CommandError,
		CommandName: c.Name(),
		Output:      wrapped.Error(),
		Err:         wrapped,
	}
}

// decodePreviousOutput mirrors how the REST binding treats a prior result: a
// JSON object decodes to its fields, and anything else is addressable under
// "output" so a plain-text producer still has one selectable path.
func decodePreviousOutput(raw string) map[string]interface{} {
	output := map[string]interface{}{}
	if raw == "" {
		return output
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return map[string]interface{}{"output": raw}
	}
	return output
}
