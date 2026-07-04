// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"testing"
)

func TestAuditDesignPatternsDelegatesToBuild(t *testing.T) {
	called := false

	err := auditDesignPatterns(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("auditDesignPatterns returned error: %v", err)
	}
	if !called {
		t.Fatal("auditDesignPatterns did not call build")
	}
}

func TestAuditDesignPatternsReturnsBuildError(t *testing.T) {
	want := errors.New("build failed")

	err := auditDesignPatterns(func() error {
		return want
	})

	if !errors.Is(err, want) {
		t.Fatalf("auditDesignPatterns error = %v, want %v", err, want)
	}
}
