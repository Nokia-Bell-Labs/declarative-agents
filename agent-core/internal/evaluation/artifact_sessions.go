// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ListEvaluationSessions lists every evaluation session under dataDir. The
// three-level directory descent (suite/timestamp/point) is externalized to a
// single find that lists each point's meta.json (externalize-to-CLI-tools,
// #1384); Go only reads the listed meta files and groups them by session.
func ListEvaluationSessions(dataDir string) ([]EvaluationSessionSummary, error) {
	if info, err := os.Stat(dataDir); err != nil || !info.IsDir() {
		if err == nil || os.IsNotExist(err) {
			return []EvaluationSessionSummary{}, nil
		}
		return nil, err
	}
	metaPaths, err := findSessionMetaFiles(dataDir)
	if err != nil {
		return nil, err
	}
	byID := map[string]*EvaluationSessionSummary{}
	for _, metaPath := range metaPaths {
		suite, timestamp, ok := sessionCoordinates(dataDir, metaPath)
		if !ok {
			continue
		}
		meta, ok := readEvalMeta(metaPath)
		if !ok {
			continue
		}
		summary := ensureSessionSummary(byID, suite, timestamp)
		tallyEvaluationMeta(summary, meta)
	}
	return sortedSessionSummaries(byID), nil
}

// findSessionMetaFiles lists every point meta.json at the fixed
// suite/timestamp/point depth beneath dataDir in one find.
func findSessionMetaFiles(dataDir string) ([]string, error) {
	cmd := exec.Command("find", dataDir,
		"-mindepth", "4", "-maxdepth", "4", "-type", "f", "-name", ArtifactMeta)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list evaluation sessions in %s: %w", dataDir, err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// sessionCoordinates extracts the suite and timestamp from a meta.json path of
// the form dataDir/<suite>/<timestamp>/<point>/meta.json.
func sessionCoordinates(dataDir, metaPath string) (suite, timestamp string, ok bool) {
	rel, err := filepath.Rel(dataDir, metaPath)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 4 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func readEvalMeta(metaPath string) (EvalMeta, bool) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return EvalMeta{}, false
	}
	var meta EvalMeta
	if json.Unmarshal(data, &meta) != nil {
		return EvalMeta{}, false
	}
	return meta, true
}

func ensureSessionSummary(byID map[string]*EvaluationSessionSummary, suite, timestamp string) *EvaluationSessionSummary {
	id := suite + "/" + timestamp
	if summary := byID[id]; summary != nil {
		return summary
	}
	summary := &EvaluationSessionSummary{ID: id, Name: suite, Timestamp: timestamp}
	byID[id] = summary
	return summary
}

func tallyEvaluationMeta(summary *EvaluationSessionSummary, meta EvalMeta) {
	summary.PointCount++
	switch {
	case meta.TimedOut:
		summary.TimeoutCount++
	case meta.TestsPassed:
		summary.PassCount++
	default:
		summary.FailCount++
	}
}

func sortedSessionSummaries(byID map[string]*EvaluationSessionSummary) []EvaluationSessionSummary {
	sessions := make([]EvaluationSessionSummary, 0, len(byID))
	for _, summary := range byID {
		sessions = append(sessions, *summary)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ID > sessions[j].ID })
	return sessions
}
