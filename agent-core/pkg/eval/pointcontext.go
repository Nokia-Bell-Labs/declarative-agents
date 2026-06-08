// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"io"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// Signals emitted by the per-point experiment commands.
const (
	SigWorkspaceReady   core.Signal = "WorkspaceReady"
	SigResultsCollected core.Signal = "ResultsCollected"
	SigMetricsCollected core.Signal = "MetricsCollected"
	SigSummarized       core.Signal = "Summarized"
)

// Signals emitted by the CLI tool.
const (
	SigHarnessFinished core.Signal = "HarnessFinished"
	SigHarnessFailed   core.Signal = "HarnessFailed"
	SigHarnessTimedOut core.Signal = "HarnessTimedOut"
)

// PointContext holds shared mutable state for a single evaluation point.
// All per-point commands read and write through this struct.
type PointContext struct {
	SessionDir string
	PointID    string
	Sample     Sample
	Harness    Harness
	Model      string
	GridPoint  GridPoint
	Rep        int
	Timeout    time.Duration
	LLMTimeout time.Duration
	OllamaURL  string
	Stderr     io.Writer

	// Populated during execution
	PointDir    string
	TracePath   string
	ResultPath  string
	Tokens      int
	TestsPassed bool
	TestOutput  string
	TimedOut    bool
	ExitCode    int
	Duration    time.Duration
}
