// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

// Test runs Go unit tests for agent-profiles.
func Test() error {
	fmt.Println("running go test ./...")
	return sh.Run("go", "test", "./...")
}
