// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
	"gopkg.in/yaml.v3"
)

type sessionSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Timestamp    string `json:"timestamp"`
	PointCount   int    `json:"pointCount"`
	PassCount    int    `json:"passCount"`
	FailCount    int    `json:"failCount"`
	TimeoutCount int    `json:"timeoutCount"`
}

type sessionDetail struct {
	ID            string           `json:"id"`
	ModelStats    []modelStatJSON  `json:"modelStats"`
	SampleStats   []sampleStatJSON `json:"sampleStats"`
	TotalPoints   int              `json:"totalPoints"`
	TotalPassed   int              `json:"totalPassed"`
	TotalFailed   int              `json:"totalFailed"`
	TotalTimedOut int              `json:"totalTimedOut"`
}

type modelStatJSON struct {
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

type sampleStatJSON struct {
	Sample        string  `json:"sample"`
	Model         string  `json:"model"`
	Runs          int     `json:"runs"`
	SuccessRate   float64 `json:"successRate"`
	MeanIter      float64 `json:"meanIter"`
	MeanTokens    float64 `json:"meanTokens"`
	MeanDurationS float64 `json:"meanDurationS"`
}

type pointJSON struct {
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

type traceJSON struct {
	PointID   string         `json:"pointId"`
	Spans     []spanJSON     `json:"spans"`
	Snapshots []snapshotJSON `json:"snapshots"`
}

type spanJSON struct {
	Name       string  `json:"name"`
	StartTime  string  `json:"startTime"`
	EndTime    string  `json:"endTime"`
	DurationMs float64 `json:"durationMs"`
	ToolName   string  `json:"toolName,omitempty"`
	Signal     string  `json:"signal,omitempty"`
	TokensIn   int     `json:"tokensIn,omitempty"`
	TokensOut  int     `json:"tokensOut,omitempty"`
}

type snapshotJSON struct {
	Tool      string `json:"tool"`
	Signal    string `json:"signal"`
	Iteration int    `json:"iteration"`
	Output    string `json:"output,omitempty"`
}

func (s *Server) sessionDir(suite, ts string) string {
	cleaned := filepath.Join(s.dataDir, filepath.Clean(suite), filepath.Clean(ts))
	if !strings.HasPrefix(cleaned, s.dataDir+string(filepath.Separator)) {
		return ""
	}
	return cleaned
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	suites, err := os.ReadDir(s.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeData(w, []sessionSummary{})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read data directory")
		return
	}

	var sessions []sessionSummary
	for _, suite := range suites {
		if !suite.IsDir() {
			continue
		}
		suiteDir := filepath.Join(s.dataDir, suite.Name())
		timestamps, err := os.ReadDir(suiteDir)
		if err != nil {
			continue
		}
		for _, ts := range timestamps {
			if !ts.IsDir() {
				continue
			}
			tsDir := filepath.Join(suiteDir, ts.Name())
			summary := scanSession(suite.Name(), ts.Name(), tsDir)
			if summary.PointCount > 0 {
				sessions = append(sessions, summary)
			}
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ID > sessions[j].ID
	})

	writeData(w, sessions)
}

func scanSession(suite, ts, dir string) sessionSummary {
	s := sessionSummary{
		ID:        suite + "/" + ts,
		Name:      suite,
		Timestamp: ts,
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return s
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(dir, e.Name(), stl.ArtifactMeta)
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta struct {
			TestsPassed bool `json:"tests_passed"`
			TimedOut    bool `json:"timed_out"`
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		s.PointCount++
		switch {
		case meta.TimedOut:
			s.TimeoutCount++
		case meta.TestsPassed:
			s.PassCount++
		default:
			s.FailCount++
		}
	}

	return s
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	suite := r.PathValue("suite")
	ts := r.PathValue("ts")
	dir := s.sessionDir(suite, ts)
	if dir == "" {
		writeError(w, http.StatusBadRequest, "invalid session path")
		return
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	groups, err := stl.LoadMultiple([]string{dir})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load session")
		return
	}

	modelStats := stl.ComputeModelStats(groups)
	sampleStats := stl.ComputeDetailed(groups)

	detail := sessionDetail{
		ID:          suite + "/" + ts,
		ModelStats:  convertModelStats(modelStats),
		SampleStats: convertSampleStats(sampleStats),
	}

	for _, ms := range modelStats {
		detail.TotalPoints += ms.Runs
		detail.TotalPassed += ms.Successes
	}
	detail.TotalFailed = detail.TotalPoints - detail.TotalPassed

	for _, runs := range groups {
		for _, run := range runs {
			if run.TimedOut {
				detail.TotalTimedOut++
			}
		}
	}
	detail.TotalFailed -= detail.TotalTimedOut

	writeData(w, detail)
}

func convertModelStats(stats []stl.ModelStats) []modelStatJSON {
	out := make([]modelStatJSON, len(stats))
	for i, s := range stats {
		out[i] = modelStatJSON{
			Model:         s.Model,
			Runs:          s.Runs,
			Successes:     s.Successes,
			SuccessRate:   s.SuccessRate,
			CleanRate:     s.CleanRate,
			RecoveryRate:  s.RecoveryRate,
			StuckRate:     s.StuckRate,
			MeanIter:      s.MeanIter,
			MeanTokensIn:  s.MeanTokensIn,
			MeanTokensOut: s.MeanTokensOut,
			MeanDurationS: s.MeanDuration.Seconds(),
		}
	}
	return out
}

func convertSampleStats(rows []stl.SampleModelRow) []sampleStatJSON {
	out := make([]sampleStatJSON, len(rows))
	for i, r := range rows {
		out[i] = sampleStatJSON{
			Sample:        r.Sample,
			Model:         r.Model,
			Runs:          r.Runs,
			SuccessRate:   r.SuccessRate,
			MeanIter:      r.MeanIter,
			MeanTokens:    r.MeanTokens,
			MeanDurationS: r.MeanDuration.Seconds(),
		}
	}
	return out
}

func (s *Server) handleListPoints(w http.ResponseWriter, r *http.Request) {
	suite := r.PathValue("suite")
	ts := r.PathValue("ts")
	dir := s.sessionDir(suite, ts)
	if dir == "" {
		writeError(w, http.StatusBadRequest, "invalid session path")
		return
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	groups, err := stl.LoadMultiple([]string{dir})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load session")
		return
	}

	runIndex := make(map[string]stl.EvalRunResult)
	for _, runs := range groups {
		for _, run := range runs {
			key := stl.EvalPointID(run.Sample, "", run.Model, nil, run.Repetition)
			runIndex[key] = run
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read session directory")
		return
	}

	var points []pointJSON
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(dir, e.Name(), stl.ArtifactMeta)
		if _, err := os.Stat(metaPath); err != nil {
			continue
		}

		pid := e.Name()
		p := pointJSON{PointID: pid}

		if run, ok := findRun(runIndex, groups, pid); ok {
			conv := string(stl.NoData)
			if run.Progression != nil {
				conv = string(run.Progression.Overall)
			}
			p.Sample = run.Sample
			p.Model = run.Model
			p.TestsPassed = run.TestsPassed
			p.TimedOut = run.TimedOut
			p.ExitCode = run.ExitCode
			p.DurationS = run.Duration.Seconds()
			p.Iterations = run.Iterations
			p.TokensIn = run.TokensIn
			p.TokensOut = run.TokensOut
			p.Convergence = conv
		}

		points = append(points, p)
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].PointID < points[j].PointID
	})

	writeData(w, points)
}

func findRun(index map[string]stl.EvalRunResult, groups map[stl.GroupKey][]stl.EvalRunResult, dirName string) (stl.EvalRunResult, bool) {
	if r, ok := index[dirName]; ok {
		return r, true
	}
	for _, runs := range groups {
		for _, r := range runs {
			candidate := stl.EvalPointID(r.Sample, "", r.Model, nil, r.Repetition)
			if candidate == dirName {
				return r, true
			}
		}
	}
	return stl.EvalRunResult{}, false
}

func (s *Server) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	suite := r.PathValue("suite")
	ts := r.PathValue("ts")
	pointID := r.PathValue("pointId")

	dir := s.sessionDir(suite, ts)
	if dir == "" {
		writeError(w, http.StatusBadRequest, "invalid session path")
		return
	}

	cleanPoint := filepath.Clean(pointID)
	if strings.Contains(cleanPoint, string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "invalid point ID")
		return
	}

	tracePath := filepath.Join(dir, cleanPoint, stl.ArtifactTrace)
	if _, err := os.Stat(tracePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	spans, err := stl.ReadTraceFile(tracePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read trace")
		return
	}

	snapshots := stl.ExtractToolSnapshots(spans)

	trace := traceJSON{
		PointID:   pointID,
		Spans:     convertSpans(spans),
		Snapshots: convertSnapshots(snapshots),
	}

	writeData(w, trace)
}

func convertSpans(spans []*stl.Span) []spanJSON {
	out := make([]spanJSON, len(spans))
	for i, s := range spans {
		dur := s.EndTime.Sub(s.StartTime)
		out[i] = spanJSON{
			Name:       s.Name,
			StartTime:  s.StartTime.Format("2006-01-02T15:04:05Z07:00"),
			EndTime:    s.EndTime.Format("2006-01-02T15:04:05Z07:00"),
			DurationMs: float64(dur.Milliseconds()),
			ToolName:   stl.StrAttr(s, "command.name"),
			Signal:     stl.StrAttr(s, "command.signal"),
			TokensIn:   stl.IntAttr(s, "gen_ai.usage.input_tokens"),
			TokensOut:  stl.IntAttr(s, "gen_ai.usage.output_tokens"),
		}
	}
	return out
}

func convertSnapshots(snaps []stl.ToolSnapshot) []snapshotJSON {
	out := make([]snapshotJSON, len(snaps))
	for i, s := range snaps {
		out[i] = snapshotJSON{
			Tool:      s.Tool,
			Signal:    s.Signal,
			Iteration: i + 1,
		}
	}
	return out
}

func (s *Server) handleGetExperiment(w http.ResponseWriter, r *http.Request) {
	suite := r.PathValue("suite")
	ts := r.PathValue("ts")
	pointID := r.PathValue("pointId")

	dir := s.sessionDir(suite, ts)
	if dir == "" {
		writeError(w, http.StatusBadRequest, "invalid session path")
		return
	}

	cleanPoint := filepath.Clean(pointID)
	if strings.Contains(cleanPoint, string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "invalid point ID")
		return
	}

	expPath := filepath.Join(dir, cleanPoint, stl.ArtifactExperiment)
	data, err := os.ReadFile(expPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "experiment.yaml not found for this point")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read experiment config")
		return
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse experiment config")
		return
	}

	writeData(w, config)
}
