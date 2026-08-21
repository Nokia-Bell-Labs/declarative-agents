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
func Test() error {
	fmt.Println("running go test -short -timeout 5m ./...")
	return sh.Run("go", "test", "-short", "-timeout", "5m", "./...")
}

// TestFull runs every agent-core Go test, including those that skip under -short.
func TestFull() error {
	fmt.Println("running go test -timeout 20m ./...")
	return sh.Run("go", "test", "-timeout", "20m", "./...")
}
