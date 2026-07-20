// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
	"github.com/stretchr/testify/require"
)

func TestRipgrepArgsNoPath(t *testing.T) {
	t.Parallel()
	f := &findCmd{root: "/work", query: `\.go$`}
	args, dir, err := f.ripgrepArgs()
	require.NoError(t, err)
	require.Equal(t, []string{"--no-heading", "--line-number", `\.go$`}, args)
	require.Equal(t, "/work", dir, "with no search path the run is scoped by cwd = root")
}

func TestRipgrepArgsWithValidSubpath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	f := &findCmd{root: root, query: "func main", searchPath: "sub"}
	args, dir, err := f.ripgrepArgs()
	require.NoError(t, err)
	require.Empty(t, dir, "a scoped path is passed as an arg, not as cwd")
	// ValidatePath resolves symlinks (macOS /var -> /private/var), so derive the
	// expected resolved path the same way rather than joining the raw root.
	resolved, verr := ValidatePath(root, "sub")
	require.NoError(t, verr)
	require.Equal(t, []string{"--no-heading", "--line-number", "func main", resolved}, args)
}

func TestRipgrepArgsRejectsEscapingPath(t *testing.T) {
	t.Parallel()
	f := &findCmd{root: t.TempDir(), query: "x", searchPath: "../../etc"}
	_, _, err := f.ripgrepArgs()
	require.Error(t, err, "a path escaping the root must be rejected before any subprocess runs")
}

func TestFindErrorSignalMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		res        *subprocess.Result
		wantSignal core.Signal
		wantSubstr string
	}{
		{"no matches is a clean empty result", &subprocess.Result{ExitCode: 1}, core.ToolDone, ""},
		{"a real ripgrep error fails", &subprocess.Result{ExitCode: 2, Stdout: "regex parse error"}, core.ToolFailed, "exit 2"},
		{"a timeout fails", &subprocess.Result{ExitCode: -1, TimedOut: true}, core.ToolFailed, "timed out"},
		{"a spawn failure fails", &subprocess.Result{ExitCode: -1, Err: os.ErrNotExist}, core.ToolFailed, "find error"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := findError(tc.res, strings.TrimRight(tc.res.Stdout, "\n"))
			require.Equal(t, tc.wantSignal, got.Signal)
			require.Equal(t, "find", got.CommandName)
			if tc.wantSubstr != "" {
				require.Contains(t, got.Output, tc.wantSubstr)
			} else {
				require.Empty(t, got.Output)
			}
		})
	}
}

func TestFindBuilderMissingQuery(t *testing.T) {
	t.Parallel()
	b := &FindBuilder{Root: "/work"}
	cmd := b.Build(core.Result{Output: `{}`})
	res := cmd.Execute()
	require.Equal(t, core.ToolFailed, res.Signal)
	require.Contains(t, res.Output, "query")
}

func TestFindBuilderDefaultsOutputCap(t *testing.T) {
	t.Parallel()
	b := &FindBuilder{Root: "/work"}
	cmd := b.Build(core.Result{Output: `{"parameters":{"query":"x"}}`})
	fc, ok := cmd.(*findCmd)
	require.True(t, ok)
	require.Equal(t, "x", fc.query)
	require.Equal(t, defaultOutputLineCap, fc.outputLineCap, "an unset cap falls back to the default")
}

func TestCapOutputTruncatesToLineCap(t *testing.T) {
	t.Parallel()
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "MATCH"
	}
	capped := capOutput(strings.Join(lines, "\n"), 3)
	require.Equal(t, 3, strings.Count(capped, "MATCH"), "output is capped to the requested line count")
	require.Contains(t, capped, "7 lines omitted", "the cap notes how many lines it dropped")
}

func TestCapOutputBelowCapUnchanged(t *testing.T) {
	t.Parallel()
	out := "a\nb"
	require.Equal(t, out, capOutput(out, 5), "output at or under the cap is returned unchanged")
}

// TestFindExecuteThroughSubprocess drives the real ripgrep run end to end through the
// shared subprocess support: a match returns ToolDone with the line, and a no-match
// query returns ToolDone with empty output (ripgrep exit 1, not a failure).
func TestFindExecuteThroughSubprocess(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not on PATH")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "sample.go"), []byte("func main() {}\n"), 0o644))

	hit := (&findCmd{root: root, query: "func main", outputLineCap: defaultOutputLineCap}).Execute()
	require.Equal(t, core.ToolDone, hit.Signal)
	require.Contains(t, hit.Output, "func main")
	require.Contains(t, hit.Output, "sample.go")

	miss := (&findCmd{root: root, query: "nonexistent_symbol_xyz", outputLineCap: defaultOutputLineCap}).Execute()
	require.Equal(t, core.ToolDone, miss.Signal, "no matches is a clean result, not a failure")
	require.Empty(t, miss.Output)
}
