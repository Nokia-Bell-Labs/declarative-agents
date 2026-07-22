// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEvaluationResults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		points      []bool
		mutate      func(*testing.T, string)
		wantPassed  int
		wantTotal   int
		wantErrText string
	}{
		{name: "success", points: []bool{true}, wantPassed: 1, wantTotal: 1},
		{name: "partial success", points: []bool{false, true}, wantPassed: 1, wantTotal: 2},
		{name: "all fail", points: []bool{false, false}, wantTotal: 2, wantErrText: "all 2 evaluation points failed"},
		{name: "missing metadata", wantErrText: "no valid point metadata"},
		{name: "malformed metadata", points: []bool{true}, mutate: func(t *testing.T, root string) {
			requireWriteResultFile(t, filepath.Join(root, "point-0", "meta.json"), "{broken")
		}, wantErrText: "parse "},
		{name: "missing trace", points: []bool{true}, mutate: func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "point-0", "trace.ndjson")); err != nil {
				t.Fatalf("remove trace: %v", err)
			}
		}, wantPassed: 1, wantTotal: 1, wantErrText: "missing trace.ndjson"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for index, passed := range tt.points {
				writeResultPoint(t, root, index, passed)
			}
			if tt.mutate != nil {
				tt.mutate(t, root)
			}
			passed, total, err := validateEvaluationResults(root)
			if passed != tt.wantPassed || total != tt.wantTotal {
				t.Fatalf("validateEvaluationResults() = %d/%d, want %d/%d", passed, total, tt.wantPassed, tt.wantTotal)
			}
			if tt.wantErrText == "" {
				if err != nil {
					t.Fatalf("validateEvaluationResults() error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("validateEvaluationResults() error = %v, want text %q", err, tt.wantErrText)
			}
		})
	}
}

func writeResultPoint(t *testing.T, root string, index int, passed bool) {
	t.Helper()
	point := filepath.Join(root, fmt.Sprintf("point-%d", index))
	if err := os.Mkdir(point, 0o700); err != nil {
		t.Fatalf("create point: %v", err)
	}
	requireWriteResultFile(t, filepath.Join(point, "meta.json"), fmt.Sprintf(
		`{"sample":"sample-%d","model":"model","tests_passed":%t}`, index, passed,
	))
	requireWriteResultFile(t, filepath.Join(point, "experiment.yaml"), "profile: test\n")
	requireWriteResultFile(t, filepath.Join(point, "trace.ndjson"), "{}\n")
}

func requireWriteResultFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
