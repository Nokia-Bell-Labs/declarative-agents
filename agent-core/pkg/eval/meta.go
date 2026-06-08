// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
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

func copyDirTo(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("cp", "-a", src+"/.", dst)
	return cmd.Run()
}

func gitInit(ctx context.Context, dir string) error {
	if _, err := stl.RunGit(ctx, dir, "init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if _, err := stl.RunGit(ctx, dir, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	cmd := exec.Command("git", "commit", "-m", "initial workspace", "--allow-empty")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=evaluator",
		"GIT_AUTHOR_EMAIL=evaluator@local",
		"GIT_COMMITTER_NAME=evaluator",
		"GIT_COMMITTER_EMAIL=evaluator@local",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", string(out), err)
	}
	return nil
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
