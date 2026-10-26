// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckoutsFileLastsOnlyForTheRun(t *testing.T) {
	root := t.TempDir()
	restore, err := withCheckouts(root, "/cache/src", parentRepository)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, checkoutsFile))
	if !strings.Contains(string(b), parentRepository+": /cache/src") {
		t.Fatalf("checkouts during the run:\n%s", b)
	}
	restore()
	if _, err := os.Stat(filepath.Join(root, checkoutsFile)); !os.IsNotExist(err) {
		t.Fatalf("the checkouts file outlived the run: %v", err)
	}
	writeFile(t, filepath.Join(root, checkoutsFile), "checkouts: {}\n")
	restore, _ = withCheckouts(root, "/cache/src", parentRepository)
	restore()
	if b, _ := os.ReadFile(filepath.Join(root, checkoutsFile)); string(b) != "checkouts: {}\n" {
		t.Fatalf("a person's checkouts file was changed: %q", b)
	}
}
