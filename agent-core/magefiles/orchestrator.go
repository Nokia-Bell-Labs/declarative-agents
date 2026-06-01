// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
)

// Cobbler groups the measure and stitch targets.
type Cobbler mg.Namespace

// Generator groups the code-generation trail lifecycle targets.
type Generator mg.Namespace

// Scaffold groups the scaffold install/uninstall targets.
type Scaffold mg.Namespace

// Prompt groups prompt preview targets.
type Prompt mg.Namespace

// Stats groups the stats targets (LOC, tokens).
type Stats mg.Namespace

// Tests: run directly with go test:
//   go test -tags=usecase -v -count=1 -timeout 1800s ./tests/rel01.0/...          # all
//   go test -tags=usecase -v ./tests/rel01.0/uc001/                               # one UC
//   go test -tags=usecase -bench=. -benchtime=1x -run=^$ ./tests/rel01.0/uc008/   # benchmarks

// baseCfg holds the configuration loaded from configuration.yaml.
var baseCfg orchestrator.Config

func init() {
	if _, err := os.Stat(orchestrator.DefaultConfigFile); errors.Is(err, os.ErrNotExist) {
		if err := orchestrator.WriteDefaultConfig(orchestrator.DefaultConfigFile); err != nil {
			panic(fmt.Sprintf("creating %s: %v", orchestrator.DefaultConfigFile, err))
		}
		fmt.Fprintf(os.Stderr, "created default %s\n", orchestrator.DefaultConfigFile)
	}
	var err error
	baseCfg, err = orchestrator.LoadConfig(orchestrator.DefaultConfigFile)
	if err != nil {
		panic(fmt.Sprintf("loading %s: %v", orchestrator.DefaultConfigFile, err))
	}
}

// newOrch creates an Orchestrator from the base config.
func newOrch() *orchestrator.Orchestrator {
	return orchestrator.New(baseCfg)
}

// logf prints a timestamped log line to stderr.
func logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] %s\n", time.Now().Format(time.RFC3339), msg)
}

// --- Top-level targets ---

// Init initializes the project.
func Init() error { return newOrch().Generator.Init() }

// Reset performs a full reset: cobbler and generator.
func Reset() error { return newOrch().Generator.FullReset() }

// Build compiles the project binary.
func Build() error { return newOrch().Builder.Build() }

// Lint runs golangci-lint on the project.
func Lint() error { return newOrch().Builder.Lint() }

// Install runs go install for the main package.
func Install() error { return newOrch().Builder.Install() }

// Clean removes build artifacts.
func Clean() error { return newOrch().Builder.Clean() }

// Analyze performs cross-artifact consistency checks (SRDs, use cases, test suites, roadmap).
func Analyze() error { return newOrch().Analyzer.Analyze() }

// Tag creates a documentation release tag (v0.YYYYMMDD.N) and builds the container image.
func Tag() error { return newOrch().Releaser.Tag() }

// --- Scaffold targets ---

// Pop removes orchestrator-managed files from the target repository:
// magefiles/orchestrator.go, docs/constitutions/, docs/prompts/, and
// configuration.yaml. Pass "." for the current directory.
func (Scaffold) Pop(target string) error { return newOrch().Scaffolder.Uninstall(target) }

// --- Cobbler targets ---

// Measure assesses project state and proposes new tasks via Claude.
func (Cobbler) Measure() error { return newOrch().Measure.Measure() }

// Stitch picks ready tasks and invokes Claude to execute them.
func (Cobbler) Stitch() error { return newOrch().Stitch.Stitch() }

// Reset removes the cobbler scratch directory.
func (Cobbler) Reset() error { return newOrch().ClaudeRunner.CobblerReset() }

// --- Generator targets ---

// Start begins a new generation trail.
func (Generator) Start() error { return newOrch().Generator.GeneratorStart() }

// Run executes measure + stitch cycles using the generation.cycles value in configuration.yaml.
// Use RunN to override the cycle count for a single invocation.
func (Generator) Run() error { return newOrch().Generator.GeneratorRun(0) }

// RunN executes exactly n cycles of measure + stitch within the current generation.
// Pass n > 0 to override generation.cycles in configuration.yaml for this run only.
func (Generator) RunN(n int) error { return newOrch().Generator.GeneratorRun(n) }

// Resume recovers from an interrupted run and continues.
func (Generator) Resume() error { return newOrch().Generator.GeneratorResume() }

// Stop completes a generation trail and merges it into main.
func (Generator) Stop() error { return newOrch().Generator.GeneratorStop() }

// List shows active branches and past generations.
func (Generator) List() error { return newOrch().Generator.GeneratorList() }

// Switch commits current work and checks out another generation branch.
func (Generator) Switch() error { return newOrch().Generator.GeneratorSwitch() }

// Reset destroys generation branches, worktrees, and Go source directories.
func (Generator) Reset() error { return newOrch().Generator.GeneratorReset() }

// --- Stats targets ---

// Loc prints Go lines of code and documentation word counts.
func (Stats) Loc() error { return newOrch().Stats.PrintStats() }

// Tokens enumerates prompt-attached files and counts tokens via the Anthropic API.
func (Stats) Tokens() error { return newOrch().TokenStats() }

// Generator prints a status report for the current generation run.
func (Stats) Generator() error { return newOrch().Stats.GeneratorStats() }

// --- Prompt targets ---

// Measure prints the assembled measure prompt to stdout.
func (Prompt) Measure() error { return newOrch().DumpMeasurePrompt() }

// Stitch prints the assembled stitch prompt to stdout.
func (Prompt) Stitch() error { return newOrch().DumpStitchPrompt() }

