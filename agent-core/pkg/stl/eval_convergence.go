// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"strings"
)

// Convergence classifies how a run's tool metrics evolved.
type Convergence string

const (
	Clean      Convergence = "CLEAN"
	Converged  Convergence = "CONVERGED"
	Improving  Convergence = "IMPROVING"
	Flat       Convergence = "FLAT"
	Regressing Convergence = "REGRESSING"
	NoData     Convergence = "NO_DATA"
)

// ToolProgression tracks per-tool metrics evolution within a run.
type ToolProgression struct {
	Tool        string         `json:"tool"`
	Snapshots   []ToolSnapshot `json:"snapshots"`
	Convergence Convergence    `json:"convergence"`
	Timeline    string         `json:"timeline"`
}

// RunProgression holds the full progression analysis for a single run.
type RunProgression struct {
	Tools   []ToolProgression `json:"tools"`
	Overall Convergence       `json:"overall"`
	Summary string            `json:"summary"`
}

// Classify analyzes the sequence of tool snapshots from a run and
// returns a RunProgression with convergence classification.
func Classify(snapshots []ToolSnapshot, succeeded bool) RunProgression {
	byTool := groupSnapshotsByTool(snapshots)

	var progs []ToolProgression
	for tool, snaps := range byTool {
		tp := ToolProgression{
			Tool:      tool,
			Snapshots: snaps,
		}
		tp.Convergence = classifyTool(snaps, succeeded)
		tp.Timeline = formatTimeline(tool, snaps)
		progs = append(progs, tp)
	}

	overall := deriveOverall(progs, succeeded)

	return RunProgression{
		Tools:   progs,
		Overall: overall,
		Summary: formatSummary(progs, overall),
	}
}

func groupSnapshotsByTool(snaps []ToolSnapshot) map[string][]ToolSnapshot {
	m := make(map[string][]ToolSnapshot)
	for _, s := range snaps {
		m[s.Tool] = append(m[s.Tool], s)
	}
	return m
}

func classifyTool(snaps []ToolSnapshot, runSucceeded bool) Convergence {
	if len(snaps) == 0 {
		return NoData
	}

	hadFailure := false
	for _, s := range snaps {
		if s.Failed > 0 || s.Signal == "ToolFailed" {
			hadFailure = true
			break
		}
	}

	if !hadFailure {
		return Clean
	}

	last := snaps[len(snaps)-1]
	if last.Failed == 0 && last.Signal != "ToolFailed" && runSucceeded {
		return Converged
	}

	if len(snaps) < 2 {
		return Flat
	}

	var failedCounts []int
	for _, s := range snaps {
		if s.Failed > 0 || s.Signal == "ToolFailed" {
			fc := s.Failed
			if fc == 0 && s.Signal == "ToolFailed" {
				fc = 1
			}
			failedCounts = append(failedCounts, fc)
		}
	}

	if len(failedCounts) < 2 {
		if last.Failed == 0 && last.Signal != "ToolFailed" {
			return Improving
		}
		return Flat
	}

	first := failedCounts[0]
	lastFC := failedCounts[len(failedCounts)-1]

	if lastFC < first {
		if lastFC == 0 {
			return Converged
		}
		return Improving
	}
	if lastFC > first {
		return Regressing
	}
	return Flat
}

func deriveOverall(progs []ToolProgression, succeeded bool) Convergence {
	if len(progs) == 0 {
		if succeeded {
			return Clean
		}
		return NoData
	}

	hasFailure := false
	allClean := true
	anyRegressing := false

	for _, p := range progs {
		if p.Convergence != Clean && p.Convergence != NoData {
			allClean = false
		}
		if p.Convergence == Regressing {
			anyRegressing = true
		}
		for _, s := range p.Snapshots {
			if s.Failed > 0 || s.Signal == "ToolFailed" {
				hasFailure = true
			}
		}
	}

	if !hasFailure && allClean {
		return Clean
	}
	if succeeded {
		return Converged
	}
	if anyRegressing {
		return Regressing
	}

	for _, p := range progs {
		if p.Convergence == Improving {
			return Improving
		}
	}

	return Flat
}

func formatTimeline(tool string, snaps []ToolSnapshot) string {
	var parts []string
	for _, s := range snaps {
		switch {
		case s.Signal == "ToolFailed" && s.Total == 0:
			parts = append(parts, "BUILD_FAIL")
		case s.Failed > 0:
			parts = append(parts, fmt.Sprintf("%dok/%dfail", s.Passed, s.Failed))
		default:
			parts = append(parts, "PASS")
		}
	}
	return strings.Join(parts, " → ")
}

func formatSummary(progs []ToolProgression, overall Convergence) string {
	if overall == Clean {
		return "clean run, no tool failures"
	}
	if overall == NoData {
		return "no metric data"
	}

	var parts []string
	for _, p := range progs {
		if p.Convergence != Clean && p.Convergence != NoData {
			parts = append(parts, fmt.Sprintf("%s:%s(%s)", p.Tool, p.Convergence, p.Timeline))
		}
	}
	return strings.Join(parts, " | ")
}
