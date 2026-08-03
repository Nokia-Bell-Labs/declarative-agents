// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	SignalEvaluationDataReady core.Signal = "EvaluationDataReady"
	SignalEvaluationMissing   core.Signal = "EvaluationMissing"
	SignalEvaluationDenied    core.Signal = "EvaluationDenied"
)

// EvaluationArtifactBuilder constructs one read-only query over evaluator
// artifacts. It contains no HTTP routing or profile workflow policy.
type EvaluationArtifactBuilder struct {
	Name      string
	Operation string
	DataDir   string
}

func (b *EvaluationArtifactBuilder) Build(res core.Result) core.Command {
	params := decodeArtifactParams(res.Output)
	return &evaluationArtifactCmd{
		name: b.Name, operation: b.Operation, dataDir: b.DataDir,
		suite: stringParam(params, "suite"), timestamp: stringParam(params, "timestamp"),
		pointID: stringParam(params, "point_id"),
	}
}

type evaluationArtifactCmd struct {
	name, operation, dataDir, suite, timestamp, pointID string
}

func (c *evaluationArtifactCmd) Name() string { return c.name }
func (c *evaluationArtifactCmd) Undo(core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

func (c *evaluationArtifactCmd) Execute() core.Result {
	var (
		value any
		err   error
	)
	switch c.operation {
	case "list_evaluation_sessions":
		value, err = ListEvaluationSessions(c.dataDir)
	case "analyze_evaluation_session":
		value, err = AnalyzeEvaluationSession(c.dataDir, c.suite, c.timestamp)
	case "list_evaluation_points":
		value, err = ListEvaluationPoints(c.dataDir, c.suite, c.timestamp)
	case "read_evaluation_trace":
		value, err = ReadEvaluationTrace(c.dataDir, c.suite, c.timestamp, c.pointID)
	default:
		err = fmt.Errorf("unknown evaluation artifact operation %q", c.operation)
	}
	if err != nil {
		return artifactErrorResult(c.Name(), err)
	}
	data, err := json.Marshal(map[string]any{"data": value})
	if err != nil {
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
	}
	return core.Result{Signal: SignalEvaluationDataReady, CommandName: c.Name(), Output: string(data)}
}

type EvaluationSessionSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Timestamp    string `json:"timestamp"`
	PointCount   int    `json:"pointCount"`
	PassCount    int    `json:"passCount"`
	FailCount    int    `json:"failCount"`
	TimeoutCount int    `json:"timeoutCount"`
}

type EvaluationSessionDetail struct {
	ID            string                 `json:"id"`
	ModelStats    []EvaluationModelStat  `json:"modelStats"`
	SampleStats   []EvaluationSampleStat `json:"sampleStats"`
	TotalPoints   int                    `json:"totalPoints"`
	TotalPassed   int                    `json:"totalPassed"`
	TotalFailed   int                    `json:"totalFailed"`
	TotalTimedOut int                    `json:"totalTimedOut"`
}

type EvaluationModelStat struct {
	Model         string  `json:"model"`
	Runs          int     `json:"runs"`
	Successes     int     `json:"successes"`
	SuccessRate   float64 `json:"successRate"`
	CleanRate     float64 `json:"cleanRate"`
	RecoveryRate  float64 `json:"recoveryRate"`
	StuckRate     float64 `json:"stuckRate"`
	MeanIter      float64 `json:"meanIter"`
	MeanTokensIn  float64 `json:"meanTokensIn"`
	MeanTokensOut float64 `json:"meanTokensOut"`
	MeanDurationS float64 `json:"meanDurationS"`
}

type EvaluationSampleStat struct {
	Sample        string  `json:"sample"`
	Model         string  `json:"model"`
	Runs          int     `json:"runs"`
	SuccessRate   float64 `json:"successRate"`
	MeanIter      float64 `json:"meanIter"`
	MeanTokens    float64 `json:"meanTokens"`
	MeanDurationS float64 `json:"meanDurationS"`
}

type EvaluationPoint struct {
	PointID     string  `json:"pointId"`
	Sample      string  `json:"sample"`
	Model       string  `json:"model"`
	TestsPassed bool    `json:"testsPassed"`
	TimedOut    bool    `json:"timedOut"`
	ExitCode    int     `json:"exitCode"`
	DurationS   float64 `json:"durationS"`
	Iterations  int     `json:"iterations"`
	TokensIn    int     `json:"tokensIn"`
	TokensOut   int     `json:"tokensOut"`
	Convergence string  `json:"convergence"`
}

type EvaluationTrace struct {
	PointID   string                   `json:"pointId"`
	Spans     []EvaluationTraceSpan    `json:"spans"`
	Snapshots []EvaluationToolSnapshot `json:"snapshots"`
}

type EvaluationTraceSpan struct {
	Name       string  `json:"name"`
	StartTime  string  `json:"startTime"`
	EndTime    string  `json:"endTime"`
	DurationMs float64 `json:"durationMs"`
	ToolName   string  `json:"toolName,omitempty"`
	Signal     string  `json:"signal,omitempty"`
	TokensIn   int     `json:"tokensIn,omitempty"`
	TokensOut  int     `json:"tokensOut,omitempty"`
}

type EvaluationToolSnapshot struct {
	Tool      string `json:"tool"`
	Signal    string `json:"signal"`
	Iteration int    `json:"iteration"`
	Output    string `json:"output,omitempty"`
}

func AnalyzeEvaluationSession(dataDir, suite, timestamp string) (EvaluationSessionDetail, error) {
	dir, err := evaluationSessionDir(dataDir, suite, timestamp)
	if err != nil {
		return EvaluationSessionDetail{}, err
	}
	groups, err := LoadMultiple([]string{dir})
	if err != nil {
		return EvaluationSessionDetail{}, err
	}
	modelStats := ComputeModelStats(groups)
	sampleStats := ComputeDetailed(groups)
	detail := EvaluationSessionDetail{
		ID:          suite + "/" + timestamp,
		ModelStats:  evaluationModelStats(modelStats),
		SampleStats: evaluationSampleStats(sampleStats),
	}
	for _, stat := range modelStats {
		detail.TotalPoints += stat.Runs
		detail.TotalPassed += stat.Successes
	}
	detail.TotalTimedOut = evaluationTimeoutCount(groups)
	detail.TotalFailed = detail.TotalPoints - detail.TotalPassed - detail.TotalTimedOut
	return detail, nil
}

func evaluationModelStats(stats []ModelStats) []EvaluationModelStat {
	result := make([]EvaluationModelStat, len(stats))
	for i, stat := range stats {
		result[i] = EvaluationModelStat{
			Model: stat.Model, Runs: stat.Runs, Successes: stat.Successes,
			SuccessRate: stat.SuccessRate, CleanRate: stat.CleanRate,
			RecoveryRate: stat.RecoveryRate, StuckRate: stat.StuckRate,
			MeanIter: stat.MeanIter, MeanTokensIn: stat.MeanTokensIn,
			MeanTokensOut: stat.MeanTokensOut, MeanDurationS: stat.MeanDuration.Seconds(),
		}
	}
	return result
}

func evaluationSampleStats(stats []SampleModelRow) []EvaluationSampleStat {
	result := make([]EvaluationSampleStat, len(stats))
	for i, stat := range stats {
		result[i] = EvaluationSampleStat{
			Sample: stat.Sample, Model: stat.Model, Runs: stat.Runs,
			SuccessRate: stat.SuccessRate, MeanIter: stat.MeanIter,
			MeanTokens: stat.MeanTokens, MeanDurationS: stat.MeanDuration.Seconds(),
		}
	}
	return result
}

func evaluationTimeoutCount(groups map[GroupKey][]EvalRunResult) int {
	count := 0
	for _, runs := range groups {
		for _, run := range runs {
			if run.TimedOut {
				count++
			}
		}
	}
	return count
}

func ListEvaluationPoints(dataDir, suite, timestamp string) ([]EvaluationPoint, error) {
	dir, err := evaluationSessionDir(dataDir, suite, timestamp)
	if err != nil {
		return nil, err
	}
	groups, err := LoadMultiple([]string{dir})
	if err != nil {
		return nil, err
	}
	runs := indexEvaluationRuns(groups)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	points := make([]EvaluationPoint, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		point, ok := evaluationPointFromEntry(dir, entry.Name(), runs)
		if !ok {
			continue
		}
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].PointID < points[j].PointID })
	return points, nil
}

func indexEvaluationRuns(groups map[GroupKey][]EvalRunResult) map[string]EvalRunResult {
	runs := make(map[string]EvalRunResult)
	for _, group := range groups {
		for _, run := range group {
			runs[evaluationRunKey(run.Sample, run.Model, run.Repetition)] = run
		}
	}
	return runs
}

func evaluationPointFromEntry(dir, pointID string, runs map[string]EvalRunResult) (EvaluationPoint, bool) {
	var meta EvalMeta
	metaData, err := os.ReadFile(filepath.Join(dir, pointID, ArtifactMeta))
	if err != nil || json.Unmarshal(metaData, &meta) != nil {
		return EvaluationPoint{}, false
	}
	point := EvaluationPoint{PointID: pointID}
	run, ok := runs[evaluationRunKey(meta.Sample, meta.Model, meta.Repetition)]
	if !ok {
		return point, true
	}
	point.Sample, point.Model = run.Sample, run.Model
	point.TestsPassed, point.TimedOut, point.ExitCode = run.TestsPassed, run.TimedOut, run.ExitCode
	point.DurationS, point.Iterations = run.Duration.Seconds(), run.Iterations
	point.TokensIn, point.TokensOut = run.TokensIn, run.TokensOut
	point.Convergence = string(NoData)
	if run.Progression != nil {
		point.Convergence = string(run.Progression.Overall)
	}
	return point, true
}

func evaluationRunKey(sample, model string, repetition int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", sample, model, repetition)
}

func ReadEvaluationTrace(dataDir, suite, timestamp, pointID string) (EvaluationTrace, error) {
	dir, err := evaluationSessionDir(dataDir, suite, timestamp)
	if err != nil {
		return EvaluationTrace{}, err
	}
	if !safeEvaluationComponent(pointID) {
		return EvaluationTrace{}, fmt.Errorf("denied evaluation point path")
	}
	// Confine the resolved trace path within the session directory before
	// reading. safeEvaluationComponent is a lexical guard only; os.ReadFile and
	// os.Stat follow symlinks, so an in-tree symlink at the point or file level
	// could otherwise escape the results root (GH-1358).
	tracePath, err := confineWithinRoot(dir, filepath.Join(dir, pointID, ArtifactTrace))
	if err != nil {
		if os.IsNotExist(err) {
			return EvaluationTrace{}, os.ErrNotExist
		}
		return EvaluationTrace{}, err
	}
	spans, err := ReadTraceFile(tracePath)
	if err != nil {
		if os.IsNotExist(err) {
			return EvaluationTrace{}, os.ErrNotExist
		}
		return EvaluationTrace{}, err
	}
	return evaluationTraceFromSpans(pointID, spans), nil
}

// evaluationTraceFromSpans projects raw trace spans onto the read-only trace
// view: per-span timing and gen_ai token attributes, plus the derived tool
// snapshots. Kept separate so ReadEvaluationTrace stays read + confine +
// delegate (GH-1398).
func evaluationTraceFromSpans(pointID string, spans []*Span) EvaluationTrace {
	snapshots := ExtractToolSnapshots(spans)
	result := EvaluationTrace{
		PointID:   pointID,
		Spans:     make([]EvaluationTraceSpan, len(spans)),
		Snapshots: make([]EvaluationToolSnapshot, len(snapshots)),
	}
	for i, span := range spans {
		result.Spans[i] = EvaluationTraceSpan{
			Name: span.Name, StartTime: span.StartTime.Format(time.RFC3339),
			EndTime:    span.EndTime.Format(time.RFC3339),
			DurationMs: float64(span.EndTime.Sub(span.StartTime).Milliseconds()),
			ToolName:   StrAttr(span, "command.name"), Signal: StrAttr(span, "command.signal"),
			TokensIn:  IntAttr(span, "gen_ai.usage.input_tokens"),
			TokensOut: IntAttr(span, "gen_ai.usage.output_tokens"),
		}
	}
	for i, snapshot := range snapshots {
		result.Snapshots[i] = EvaluationToolSnapshot{Tool: snapshot.Tool, Signal: snapshot.Signal, Iteration: i + 1}
	}
	return result
}

func evaluationSessionDir(dataDir, suite, timestamp string) (string, error) {
	if !safeEvaluationComponent(suite) || !safeEvaluationComponent(timestamp) {
		return "", fmt.Errorf("denied evaluation session path")
	}
	dir := filepath.Join(dataDir, suite, timestamp)
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return "", os.ErrNotExist
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", os.ErrNotExist
	}
	// os.Stat above followed symlinks, so a symlinked suite or timestamp
	// component can point outside dataDir. Resolve the path and require it to
	// remain within the results root before any reader uses it (GH-1358).
	return confineWithinRoot(dataDir, dir)
}

func safeEvaluationComponent(value string) bool {
	return value != "" && value != "." && value != ".." &&
		!strings.ContainsAny(value, `/\`+"\x00")
}

// confineWithinRoot resolves the symlinks in target and returns the resolved
// path only if it stays within root after resolution. root must exist; a
// nonexistent target propagates os.ErrNotExist so callers keep their
// not-found behavior. An escape returns a "denied evaluation" error, which the
// command layer maps to SignalEvaluationDenied.
func confineWithinRoot(root, target string) (string, error) {
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	if !pathWithin(rootResolved, resolved) {
		return "", fmt.Errorf("denied evaluation path: %q escapes results root", target)
	}
	return resolved, nil
}

// pathWithin reports whether p is root itself or lies beneath it.
func pathWithin(root, p string) bool {
	if p == root {
		return true
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func decodeArtifactParams(raw string) map[string]any {
	var params map[string]any
	if json.Unmarshal([]byte(raw), &params) != nil {
		return map[string]any{}
	}
	if nested, ok := params["parameters"].(map[string]any); ok {
		return nested
	}
	return params
}

func stringParam(params map[string]any, name string) string {
	value, _ := params[name].(string)
	return value
}

func artifactErrorResult(name string, err error) core.Result {
	signal := core.CommandError
	switch {
	case os.IsNotExist(err):
		signal = SignalEvaluationMissing
	case strings.Contains(err.Error(), "denied evaluation"):
		signal = SignalEvaluationDenied
	}
	return core.Result{Signal: signal, CommandName: name, Output: err.Error(), Err: err}
}
