// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EvalMeta records metadata for a single evaluation point.
type EvalMeta struct {
	Harness     string                 `json:"harness"`
	Model       string                 `json:"model"`
	Sample      string                 `json:"sample"`
	GridParams  map[string]interface{} `json:"grid_params"`
	Repetition  int                    `json:"repetition"`
	ExitCode    int                    `json:"exit_code"`
	Duration    time.Duration          `json:"duration_ns"`
	TestsPassed bool                   `json:"tests_passed"`
	TestOutput  string                 `json:"test_output"`
	TimedOut    bool                   `json:"timed_out"`
}

// EvalPointID produces the directory name for a single evaluation point.
func EvalPointID(sample, harness, model string, gridPoint GridPoint, rep int) string {
	parts := []string{sample, harness, model}
	gridStr := FormatGridPoint(gridPoint)
	if gridStr != "" {
		parts = append(parts, gridStr)
	}
	return fmt.Sprintf("%s--rep%d", strings.Join(parts, "--"), rep)
}

func runOracleCheck(workspaceDir string) (bool, string) {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = workspaceDir
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}

func writeMetaJSON(pc *PointContext) ([]byte, error) {
	meta := EvalMeta{
		Harness:     pc.Harness.Name,
		Model:       pc.Model,
		Sample:      pc.Sample.Name,
		GridParams:  pc.GridPoint,
		Repetition:  pc.Rep,
		ExitCode:    pc.ExitCode,
		Duration:    pc.Duration,
		TestsPassed: pc.TestsPassed,
		TestOutput:  pc.TestOutput,
		TimedOut:    pc.TimedOut,
	}

	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	metaPath := filepath.Join(pc.PointDir, "meta.json")
	if err := os.WriteFile(metaPath, metaJSON, 0o644); err != nil {
		return nil, fmt.Errorf("write meta.json: %w", err)
	}
	return metaJSON, nil
}
