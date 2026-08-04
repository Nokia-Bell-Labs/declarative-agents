// Copyright (c) 2026 Nokia. All rights reserved.

// catalog-test-evidence preserves the specification-critic's Go JSON evidence
// contract while running catalog conformance from a stable test-binary path.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newEvidenceRunner().run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
