// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureSourceCreatesImmutableWorkspaceArtifact(t *testing.T) {
	workspace := t.TempDir()
	fixtures := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixtures, "source.md"), []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var suite fixtureSuite
	suite.Source.File = "source.md"
	boundary := boundary{workspace: workspace, fixtures: fixtures, suite: suite}
	state := manifest{}

	if _, _, err := boundary.captureSource(&state, "capture", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, ".tracer", "captured-source.md")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o444 {
		t.Fatalf("capture mode = %o, want 444", got)
	}
}

func TestParseManifestRevisionInputRequiresDeclaredEventAndTerminal(t *testing.T) {
	input, err := parseManifestRevisionInput([]string{
		"boundary", "append-manifest-revision", "locally_finalized", "LocallyFinalized", "4", "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Event != "locally_finalized" || input.Terminal != "LocallyFinalized" ||
		input.Occurrence != 4 || input.ContextAttempt != 0 {
		t.Fatalf("parsed input = %#v", input)
	}
	for _, args := range [][]string{
		{"boundary", "append-manifest-revision"},
		{"boundary", "append-manifest-revision", "", "none", "1", "0"},
		{"boundary", "append-manifest-revision", "event", "Unexpected", "1", "0"},
		{"boundary", "append-manifest-revision", "event", "none", "zero", "0"},
		{"boundary", "append-manifest-revision", "event", "none", "1", "-1"},
	} {
		if _, err := parseManifestRevisionInput(args); err == nil {
			t.Fatalf("input %v unexpectedly succeeded", args)
		}
	}
}

func TestManifestContextProjectsContentIdentitiesWithoutInstructionPolicy(t *testing.T) {
	workspace := t.TempDir()
	original := []byte("original")
	candidate := []byte("candidate")
	if err := os.WriteFile(filepath.Join(workspace, "00-original.md"), original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "attempts", "structure"), 0o755); err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join("attempts", "structure", "candidate.md")
	if err := os.WriteFile(filepath.Join(workspace, candidatePath), candidate, 0o600); err != nil {
		t.Fatal(err)
	}
	state := manifest{
		SagaID: "saga", Source: sourceIdentity{SHA256: digest(original)},
		Selected: map[string]string{"original": "original-1", "structure": "structure-1"},
		Artifacts: []artifact{{
			ID: "structure-1", Stage: "structure", Attempt: 1,
			Path: filepath.ToSlash(candidatePath), SHA256: digest(candidate),
		}},
	}

	data, err := (&boundary{workspace: workspace}).manifestContext(state, 1)
	if err != nil {
		t.Fatal(err)
	}
	var context map[string]any
	if err := json.Unmarshal(data, &context); err != nil {
		t.Fatal(err)
	}
	if context["original_content"] != "original" || context["candidate_content"] != "candidate" {
		t.Fatalf("manifest context = %#v", context)
	}
	if _, exists := context["bounded_structure_intent"]; exists {
		t.Fatal("boundary context must not own child instruction policy")
	}
}

func TestChildRequestInputRequiresDistinctAddress(t *testing.T) {
	input, err := parseChildRequestInput([]string{
		"boundary", "persist-child-request", ".tracer/requests/structure-1.json", "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Path != ".tracer/requests/structure-1.json" {
		t.Fatalf("path = %q", input.Path)
	}
	for _, path := range []string{"child-request.json", "../request.json", "/tmp/request.json"} {
		if _, err := parseChildRequestInput([]string{
			"boundary", "persist-child-request", path, "1",
		}); err == nil {
			t.Fatalf("unsafe path %q succeeded", path)
		}
	}
}

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
