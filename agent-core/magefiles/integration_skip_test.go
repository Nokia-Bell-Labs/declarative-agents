// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"testing"
)

func TestIntegrationSkipStateIsInvocationLocal(t *testing.T) {
	tests := []struct {
		name  string
		steps []func()
		want  bool
	}{
		{
			name: "skipped then passed",
			steps: []func(){
				func() { _ = skipUC("probe", "missing prerequisite") },
				func() { beginUC("probe") },
			},
		},
		{
			name: "passed then skipped",
			steps: []func(){
				func() { beginUC("probe") },
				func() { _ = skipUC("probe", "missing prerequisite") },
			},
			want: true,
		},
		{
			name: "failure does not become skip",
			steps: []func(){
				func() { beginUC("probe") },
				func() { _ = errors.New("integration failed") },
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, step := range tt.steps {
				step()
			}
			if got := wasSkipped("probe"); got != tt.want {
				t.Fatalf("wasSkipped(probe) = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestIntegrationSkipStateIsKeyIsolated(t *testing.T) {
	beginUC("first")
	beginUC("second")
	_ = skipUC("first", "missing prerequisite")

	if !wasSkipped("first") {
		t.Fatal("first should be skipped")
	}
	if wasSkipped("second") {
		t.Fatal("second inherited first skip state")
	}
}
