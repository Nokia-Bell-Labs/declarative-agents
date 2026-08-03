// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
)

// The read word is the highest-frequency tool in a coding-agent loop, so the
// audit (GH-1392) asked whether the in-process builtin should be externalized
// to a CLI binding like its siblings (find -> rg, list_files -> a find-based
// exec word). The equivalent exec form is `sed -n '<start>,<end>p'` plus `nl`
// for the line-number prefixes and `file --mime-encoding` for the binary sniff.
//
// These benchmarks measure the cost the two forms would pay per read. The
// builtin path is one os.ReadFile plus line slicing; the exec floor is a single
// fork+exec of sed over the same file (the real binding would fork three times).
// Run:
//
//	go test ./internal/tools/filesystem -run '^$' \
//	  -bench 'BenchmarkRead(Builtin|ExecFloor)' -benchmem
//
// The recorded verdict lives in agent-core/tools/builtin/read.yaml: the builtin
// stays, because the fork+exec floor is orders of magnitude slower per call and
// read pays that cost on every loop iteration, where rg's binding pays it once
// per search.
func benchReadFile(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	path := filepath.Join(dir, "sample.go")
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "line %d: the quick brown fox jumps over the lazy dog\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	return path
}

// BenchmarkReadBuiltin measures the current in-process path: read the file and
// format the line-numbered range.
func BenchmarkReadBuiltin(b *testing.B) {
	path := benchReadFile(b)
	root := filepath.Dir(path)
	cmd := &readCmd{root: root, path: filepath.Base(path), startLine: 1, endLine: 200}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if res := cmd.Execute(); res.Signal == "" {
			b.Fatal("empty signal")
		}
	}
}

// BenchmarkReadExecFloor measures the lower bound of an exec binding: one
// fork+exec of sed over the same file. A faithful binding would add nl and a
// file --mime-encoding probe, so this understates the exec cost.
func BenchmarkReadExecFloor(b *testing.B) {
	path := benchReadFile(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := subprocess.Run(ctx, subprocess.Spec{
			Binary: "sed", Args: []string{"-n", "1,200p", path}, CombinedOutput: true,
		})
		if !res.Success() {
			b.Fatalf("sed failed: %v", res.Err)
		}
	}
}
