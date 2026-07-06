// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "fmt"

// Build is currently a no-op because agent-profiles has no durable build artifact.
func Build() error {
	fmt.Println("nothing to build")
	return nil
}

// Clean is currently a no-op because agent-profiles has no durable generated artifacts.
func Clean() error {
	fmt.Println("nothing to clean")
	return nil
}
