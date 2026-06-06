// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"os/exec"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// --- build tool ---

type buildCmd struct{ root string }

func (b *buildCmd) Name() string { return "build" }

func (b *buildCmd) Execute() core.Result {
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = b.root
	out, err := cmd.CombinedOutput()
	res := SubprocessResult("build", out, err)
	res.Metrics = ParseBuildMetrics(res.Output)
	return res
}

// BuildBuilder constructs build commands.
type BuildBuilder struct {
	Root string
}

func (b *BuildBuilder) Build(_ core.Result) core.Command {
	return &buildCmd{root: b.Root}
}

// BuildToolSpec returns the ToolSpec for the build tool.
func BuildToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "build",
		Description: "Compile all Go packages with go build ./... Returns compiler errors on failure.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Visibility:  core.External,
	}
}

// --- vet tool ---

type vetCmd struct{ root string }

func (v *vetCmd) Name() string { return "vet" }

func (v *vetCmd) Execute() core.Result {
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = v.root
	out, err := cmd.CombinedOutput()
	return SubprocessResult("vet", out, err)
}

// VetBuilder constructs vet commands.
type VetBuilder struct {
	Root string
}

func (b *VetBuilder) Build(_ core.Result) core.Command {
	return &vetCmd{root: b.Root}
}

// VetToolSpec returns the ToolSpec for the vet tool.
func VetToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "vet",
		Description: "Run go vet ./... on the workspace. Reports suspicious constructs.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Visibility:  core.External,
	}
}

// --- lint tool ---

type lintCmd struct{ root string }

func (l *lintCmd) Name() string { return "lint" }

func (l *lintCmd) Execute() core.Result {
	cmd := exec.Command("golangci-lint", "run", "./...")
	cmd.Dir = l.root
	out, err := cmd.CombinedOutput()
	return SubprocessResult("lint", out, err)
}

// LintBuilder constructs lint commands.
type LintBuilder struct {
	Root string
}

func (b *LintBuilder) Build(_ core.Result) core.Command {
	return &lintCmd{root: b.Root}
}

// LintToolSpec returns the ToolSpec for the lint tool.
func LintToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "lint",
		Description: "Run golangci-lint run ./... on the Go workspace.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Visibility:  core.External,
	}
}

// --- test tool ---

type testCmd struct {
	root          string
	pkg           string
	outputLineCap int
}

func (t *testCmd) Name() string { return "test" }

func (t *testCmd) Execute() core.Result {
	args := []string{"test", "-count=1", t.pkg}
	cmd := exec.Command("go", args...)
	cmd.Dir = t.root
	out, err := cmd.CombinedOutput()
	res := SubprocessResult("test", out, err)
	res.Metrics = ParseTestMetrics(res.Output)
	if t.outputLineCap > 0 {
		res.Output = CapOutput(res.Output, t.outputLineCap)
	}
	return res
}

// TestBuilder constructs test commands.
type TestBuilder struct {
	Root          string
	OutputLineCap int
}

func (b *TestBuilder) Build(res core.Result) core.Command {
	pkg := "./..."
	if p := ExtractStringParam(res.Output, "package"); p != "" {
		pkg = p
	}
	cap := b.OutputLineCap
	if cap == 0 {
		cap = DefaultOutputLineCap
	}
	return &testCmd{
		root:          b.Root,
		pkg:           pkg,
		outputLineCap: cap,
	}
}

// TestToolSpec returns the ToolSpec for the test tool.
func TestToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "test",
		Description: "Run go test -count=1 on the workspace. Returns test output including pass/fail results. Defaults to ./... (all packages).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"package":{"type":"string","description":"Go package path (default ./...)"}}}`),
		Visibility:  core.External,
	}
}
