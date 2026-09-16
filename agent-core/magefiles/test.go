// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

var Aliases = map[string]interface{}{
	"test:full": TestFull,
}

// Test runs the fast Go test suite for agent-core (`-short`).
//
// RunV rather than Run: sh.Run sends stdout to a nil writer unless mage runs
// verbose, and go test writes its failures there, so a failing lane reported
// an exit code and nothing else (GH-2055).
func Test() error {
	fmt.Println("running go test -short -timeout 5m ./...")
	return sh.RunV("go", "test", "-short", "-timeout", "5m", "./...")
}

// TestFull runs every agent-core Go test, including those that skip under -short.
func TestFull() error {
	fmt.Println("running go test -timeout 20m ./...")
	return sh.RunV("go", "test", "-timeout", "20m", "./...")
}
