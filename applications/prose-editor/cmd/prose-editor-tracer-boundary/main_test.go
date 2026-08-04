// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCriticEvaluationBindsArtifactsAndCrossFields(t *testing.T) {
	workspace := t.TempDir()
	originalBytes := []byte("immutable original\n")
	candidateBytes := []byte("structure candidate\n")
	if err := os.WriteFile(filepath.Join(workspace, "00-original.md"), originalBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "attempts", "structure"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "attempts", "structure", "candidate.md"), candidateBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	original := artifact{
		ID: "original", Stage: "original", Path: "00-original.md", SHA256: digest(originalBytes),
	}
	structure := artifact{
		ID: "structure", Stage: "structure", Attempt: 1,
		Path: "attempts/structure/candidate.md", SHA256: digest(candidateBytes),
	}
	state := manifest{
		Artifacts: []artifact{original, structure},
		Selected:  map[string]string{"original": original.ID, "structure": structure.ID},
	}
	boundary := boundary{workspace: workspace}

	tests := []struct {
		name   string
		change func(*criticEvaluation)
		want   string
	}{
		{
			name: "wrong hashes",
			change: func(evaluation *criticEvaluation) {
				evaluation.OriginalContentHash = strings.Repeat("0", 64)
				evaluation.CandidateContentHash = strings.Repeat("1", 64)
			},
			want: "original hash",
		},
		{
			name: "duplicate and missing categories",
			change: func(evaluation *criticEvaluation) {
				evaluation.Findings[5].Category = "semantic_preservation"
			},
			want: "duplicate",
		},
		{
			name: "pass with stage",
			change: func(evaluation *criticEvaluation) {
				evaluation.ResponsibleStage = "structure"
			},
			want: "must not name",
		},
		{
			name: "reject without stage",
			change: func(evaluation *criticEvaluation) {
				evaluation.Verdict = "reject"
				evaluation.Findings[0].Status = "reject"
			},
			want: "must name structure",
		},
		{
			name: "pass with rejected finding",
			change: func(evaluation *criticEvaluation) {
				evaluation.Findings[0].Status = "reject"
			},
			want: "all findings",
		},
		{
			name: "reject with passing findings",
			change: func(evaluation *criticEvaluation) {
				evaluation.Verdict = "reject"
				evaluation.ResponsibleStage = "structure"
			},
			want: "at least one",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluation := validCriticEvaluation(original.SHA256, structure.SHA256)
			test.change(&evaluation)
			err := boundary.validateCriticEvaluation(state, structure, evaluation)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want containing %q", err, test.want)
			}
		})
	}

	for _, evaluation := range []criticEvaluation{
		validCriticEvaluation(original.SHA256, structure.SHA256),
		validRejectedCriticEvaluation(original.SHA256, structure.SHA256),
	} {
		if err := boundary.validateCriticEvaluation(state, structure, evaluation); err != nil {
			t.Fatalf("valid evaluation rejected: %v", err)
		}
	}
}

func validCriticEvaluation(originalHash, candidateHash string) criticEvaluation {
	categories := []string{
		"semantic_preservation",
		"structural_intent",
		"voice_match",
		"tightening_quality",
		"unsupported_additions",
		"anchor_copy_risk",
	}
	findings := make([]criticFinding, 0, len(categories))
	for _, category := range categories {
		findings = append(findings, criticFinding{
			Category: category, Status: "pass", Summary: "bounded",
		})
	}
	return criticEvaluation{
		Verdict: "pass", OriginalContentHash: originalHash,
		CandidateContentHash: candidateHash, Findings: findings,
	}
}

func validRejectedCriticEvaluation(originalHash, candidateHash string) criticEvaluation {
	evaluation := validCriticEvaluation(originalHash, candidateHash)
	evaluation.Verdict = "reject"
	evaluation.ResponsibleStage = "structure"
	evaluation.Findings[0].Status = "reject"
	return evaluation
}
