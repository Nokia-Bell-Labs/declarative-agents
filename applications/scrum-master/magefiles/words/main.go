// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Command words is the rig's harness: the words as one command line, which
// the integration targets drive in place of a person's skill or a profile's
// run. It is never shipped; a release ships the agent, which serves the
// words in its own process (GH-37).
package main

import (
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/words"
)

func main() {
	os.Exit(words.Main(os.Args[1:], os.Stdout, os.Stderr))
}
