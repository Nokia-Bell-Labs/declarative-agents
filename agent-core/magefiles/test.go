// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

// Test runs Go unit tests for agent-core.
func Test() error {
	fmt.Println("running go test ./...")
	return sh.Run("go", "test", "./...")
}
