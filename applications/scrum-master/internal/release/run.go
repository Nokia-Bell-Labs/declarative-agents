// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// Exec runs one gate's command in dir, writing its output to out.
type Exec func(dir, command string, out io.Writer) error

// ShellExec runs the command with sh -c, as a person would type it, or a
// command line of words in process (GH-32).
func ShellExec(dir, command string, out io.Writer) error {
	if ok, err := program.RunWords(dir, command, out); ok {
		return err
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}

// GateResult is how one gate ended.
type GateResult struct {
	Stage   string
	Gate    string
	Elapsed time.Duration
	Err     error
}

// RunStages runs the stages in order and the gates of a stage at the same
// time, each line of a gate's output prefixed with its name. It stops after
// the first stage with a failed gate and returns an error naming every
// failed gate in that stage.
func RunStages(root string, g Gates, run Exec, out io.Writer) ([]GateResult, error) {
	var mu sync.Mutex
	var results []GateResult
	for _, stage := range g.Stages {
		fmt.Fprintf(out, "=== stage %s ===\n", stage.Name)
		stageResults := make([]GateResult, len(stage.Gates))
		var wg sync.WaitGroup
		for i, gate := range stage.Gates {
			wg.Add(1)
			go func() {
				defer wg.Done()
				w := &prefixWriter{prefix: "[" + gate.Name + "] ", out: out, mu: &mu}
				started := time.Now()
				err := run(filepath.Join(root, gate.Directory), gate.Command, w)
				w.flush()
				stageResults[i] = GateResult{stage.Name, gate.Name, time.Since(started).Round(time.Millisecond), err}
			}()
		}
		wg.Wait()
		results = append(results, stageResults...)
		var failed []string
		for _, r := range stageResults {
			status := "passed"
			if r.Err != nil {
				status = "FAILED: " + r.Err.Error()
				failed = append(failed, r.Gate)
			}
			fmt.Fprintf(out, "=== gate %s %s (%s) ===\n", r.Gate, status, r.Elapsed)
		}
		if len(failed) > 0 {
			return results, fmt.Errorf("stage %s failed: %s", stage.Name, strings.Join(failed, ", "))
		}
	}
	return results, nil
}

// prefixWriter writes whole lines to out under a shared lock, so gates
// running together do not interleave within a line.
type prefixWriter struct {
	prefix string
	out    io.Writer
	mu     *sync.Mutex
	buf    bytes.Buffer
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadBytes('\n')
		if err != nil {
			w.buf.Write(line)
			return len(p), nil
		}
		w.mu.Lock()
		fmt.Fprintf(w.out, "%s%s", w.prefix, line)
		w.mu.Unlock()
	}
}

func (w *prefixWriter) flush() {
	if w.buf.Len() > 0 {
		w.mu.Lock()
		fmt.Fprintf(w.out, "%s%s\n", w.prefix, w.buf.String())
		w.mu.Unlock()
		w.buf.Reset()
	}
}
