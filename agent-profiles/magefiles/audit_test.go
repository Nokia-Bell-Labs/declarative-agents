// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"testing"
)

func TestAuditProfilesDelegatesToValidate(t *testing.T) {
	called := false

	err := auditProfiles(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("auditProfiles returned error: %v", err)
	}
	if !called {
		t.Fatal("auditProfiles did not call validate")
	}
}

func TestAuditProfilesReturnsValidationError(t *testing.T) {
	want := errors.New("validation failed")

	err := auditProfiles(func() error {
		return want
	})

	if !errors.Is(err, want) {
		t.Fatalf("auditProfiles error = %v, want %v", err, want)
	}
}
