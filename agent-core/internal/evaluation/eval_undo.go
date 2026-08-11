// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type evalSessionSnapshot struct {
	Suite        SuiteConfig   `json:"suite"`
	SessionDir   string        `json:"session_dir,omitempty"`
	PointMachine string        `json:"point_machine,omitempty"`
	Result       SessionResult `json:"result"`
	PC           *PointContext `json:"point_context,omitempty"`
	GridPoints   []GridPoint   `json:"grid_points,omitempty"`
	Reps         int           `json:"reps"`
	Timeout      int64         `json:"timeout"`
	OllamaURL    string        `json:"ollama_url,omitempty"`
	LLMTimeout   int64         `json:"llm_timeout"`
	StartUnixNS  int64         `json:"start_unix_ns,omitempty"`
}

func snapshotEvalSession(es *EvalSessionState) evalSessionSnapshot {
	snap := evalSessionSnapshot{
		Suite: cloneSuiteConfig(es.Suite), SessionDir: es.SessionDir, PointMachine: es.PointMachine,
		Result: cloneSessionResult(es.Result), PC: receiptPointContext(es.PC),
		GridPoints: cloneGridPoints(es.gridPoints), Reps: es.reps, Timeout: int64(es.timeout),
		OllamaURL: es.ollamaURL, LLMTimeout: int64(es.llmTimeout),
	}
	if !es.start.IsZero() {
		snap.StartUnixNS = es.start.UnixNano()
	}
	return snap
}

func (s evalSessionSnapshot) restore(es *EvalSessionState) {
	es.Suite = cloneSuiteConfig(s.Suite)
	es.SessionDir = s.SessionDir
	es.PointMachine = s.PointMachine
	es.Result = cloneSessionResult(s.Result)
	stderr := pointStderr(es.PC)
	es.PC = clonePointContext(s.PC)
	if es.PC != nil {
		es.PC.Stderr = stderr
	}
	es.gridPoints = cloneGridPoints(s.GridPoints)
	es.reps = s.Reps
	es.timeout = time.Duration(s.Timeout)
	es.ollamaURL = s.OllamaURL
	es.llmTimeout = time.Duration(s.LLMTimeout)
	if s.StartUnixNS == 0 {
		es.start = time.Time{}
	} else {
		es.start = time.Unix(0, s.StartUnixNS)
	}
}

type pointContextSnapshot struct {
	Point *PointContext `json:"point"`
}

func snapshotPointContext(pc *PointContext) pointContextSnapshot {
	return pointContextSnapshot{Point: receiptPointContext(pc)}
}

type evaluatorReceipt struct {
	Version          int                   `json:"version"`
	Session          *evalSessionSnapshot  `json:"session,omitempty"`
	Point            *pointContextSnapshot `json:"point,omitempty"`
	RemovePaths      []string              `json:"remove_paths,omitempty"`
	RemoveRoot       string                `json:"remove_root,omitempty"`
	Boundary         string                `json:"boundary,omitempty"`
	BoundaryMetadata any                   `json:"boundary_metadata,omitempty"`
}

type evaluatorReceiptCmd struct {
	inner            core.Command
	session          *EvalSessionState
	point            *PointContext
	removePaths      func() []string
	removeRoot       func() string
	boundary         string
	boundaryMetadata func() any
}

func (c *evaluatorReceiptCmd) Name() string { return c.inner.Name() }

func (c *evaluatorReceiptCmd) SetCommandState(view core.CommandStateView) {
	if aware, ok := c.inner.(core.CommandStateAware); ok {
		aware.SetCommandState(view)
	}
}

func (c *evaluatorReceiptCmd) Execute() core.Result {
	receipt := evaluatorReceipt{Version: 1, Boundary: c.boundary}
	if c.session != nil {
		snapshot := snapshotEvalSession(c.session)
		receipt.Session = &snapshot
	}
	if c.point != nil {
		snapshot := snapshotPointContext(c.point)
		receipt.Point = &snapshot
	}
	result := c.inner.Execute()
	if c.removePaths != nil {
		receipt.RemovePaths = c.removePaths()
	}
	if c.removeRoot != nil {
		receipt.RemoveRoot = c.removeRoot()
	}
	if c.boundaryMetadata != nil {
		receipt.BoundaryMetadata = c.boundaryMetadata()
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return evaluatorReceiptError(c.Name(), fmt.Errorf("encode evaluator receipt: %w", err))
	}
	result.Receipt = string(data)
	return result
}

func (c *evaluatorReceiptCmd) Undo(prior core.Result) core.Result {
	var receipt evaluatorReceipt
	if prior.Receipt == "" {
		return evaluatorReceiptError(c.Name(), fmt.Errorf("evaluator receipt is required"))
	}
	if err := json.Unmarshal([]byte(prior.Receipt), &receipt); err != nil {
		return evaluatorReceiptError(c.Name(), fmt.Errorf("decode evaluator receipt: %w", err))
	}
	if receipt.Version != 1 {
		return evaluatorReceiptError(c.Name(), fmt.Errorf("unsupported evaluator receipt version %d", receipt.Version))
	}
	if c.session != nil && receipt.Session != nil {
		receipt.Session.restore(c.session)
	}
	if c.point != nil && receipt.Point != nil && receipt.Point.Point != nil {
		stderr := c.point.Stderr
		*c.point = *clonePointContext(receipt.Point.Point)
		c.point.Stderr = stderr
	}
	for _, path := range receipt.RemovePaths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if !ownedArtifactPath(receipt.RemoveRoot, path) {
			return evaluatorReceiptError(c.Name(), fmt.Errorf("refuse unsafe owned artifact path %q", path))
		}
		if err := os.RemoveAll(path); err != nil {
			return evaluatorReceiptError(c.Name(), fmt.Errorf("remove owned artifact %q: %w", path, err))
		}
	}
	if receipt.Boundary != "" {
		return evaluatorReceiptError(c.Name(), fmt.Errorf("boundary compensation required: %s", receipt.Boundary))
	}
	return core.Result{Signal: core.ToolDone, CommandName: c.Name(), Output: "undo: restored evaluator state and owned artifacts"}
}

func ownedArtifactPath(root, path string) bool {
	cleanRoot := filepath.Clean(strings.TrimSpace(root))
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(cleanRoot) || !filepath.IsAbs(cleanPath) {
		return false
	}
	relative, err := filepath.Rel(cleanRoot, cleanPath)
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func evaluatorReceiptError(commandName string, err error) core.Result {
	wrapped := fmt.Errorf("undo %s: %w", commandName, err)
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: wrapped.Error(), Err: wrapped}
}

func receiptPointContext(pc *PointContext) *PointContext {
	cloned := clonePointContext(pc)
	if cloned != nil {
		cloned.Stderr = nil
	}
	return cloned
}

func pointStderr(pc *PointContext) interface{ Write([]byte) (int, error) } {
	if pc == nil {
		return nil
	}
	return pc.Stderr
}

func cloneSuiteConfig(in SuiteConfig) SuiteConfig {
	out := in
	out.Profiles = append([]SuiteProfile(nil), in.Profiles...)
	out.Samples = append([]Sample(nil), in.Samples...)
	if in.Grid != nil {
		out.Grid = make(map[string][]any, len(in.Grid))
		for k, values := range in.Grid {
			out.Grid[k] = append([]any(nil), values...)
		}
	}
	return out
}

func cloneSessionResult(in SessionResult) SessionResult {
	out := in
	out.Points = append([]PointResult(nil), in.Points...)
	return out
}

func cloneGridPoints(in []GridPoint) []GridPoint {
	if in == nil {
		return nil
	}
	out := make([]GridPoint, len(in))
	for i, gp := range in {
		out[i] = cloneGridPoint(gp)
	}
	return out
}

func cloneGridPoint(in GridPoint) GridPoint {
	if in == nil {
		return nil
	}
	out := make(GridPoint, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clonePointContext(in *PointContext) *PointContext {
	if in == nil {
		return nil
	}
	out := *in
	out.GridPoint = cloneGridPoint(in.GridPoint)
	return &out
}
