// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import "fmt"

// Build is currently a no-op because the catalog has no durable build artifact.
func Build() error {
	fmt.Println("nothing to build")
	return nil
}

// Clean is currently a no-op because the catalog has no durable generated artifacts.
func Clean() error {
	fmt.Println("nothing to clean")
	return nil
}
