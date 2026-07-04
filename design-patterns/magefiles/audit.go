// Copyright (c) 2026 Nokia. All rights reserved.

package main

// Audit renders figures and builds the design-patterns PDF.
func Audit() error {
	return auditDesignPatterns(All)
}

func auditDesignPatterns(build func() error) error {
	return build()
}
