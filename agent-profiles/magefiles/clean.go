// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "fmt"

// Clean is currently a no-op because agent-profiles has no durable generated artifacts.
func Clean() error {
	fmt.Println("nothing to clean")
	return nil
}
