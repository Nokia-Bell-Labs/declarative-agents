// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

// Test runs the fast Go test suite for agent-core (`-short`).
func Test() error {
	fmt.Println("running go test -short -timeout 5m ./...")
	return sh.Run("go", "test", "-short", "-timeout", "5m", "./...")
}
