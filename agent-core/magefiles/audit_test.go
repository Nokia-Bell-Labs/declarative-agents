// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"strings"
	"testing"
)

// TestFormalEvidenceSerializesGoPackages guards the resource budget behind the
// specification-critic audit. Both inventory and uncached execution compile and
// start test binaries; package parallelism here competes with root module audits
// and makes five-second child-process tests nondeterministic (#1587).
func TestFormalEvidenceSerializesGoPackages(t *testing.T) {
	data, err := os.ReadFile("../testdata/integration/profiles/audit/go-test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "args: [test, -p=1,"); got != 2 {
		t.Fatalf("serialized go-test commands = %d, want 2", got)
	}
	if got := strings.Count(
		string(data),
		"args: [test, -p=1, -json, -count=1, ./...]",
	); got != 1 {
		t.Fatalf("full go-test evidence runs = %d, want one shared run", got)
	}
}
