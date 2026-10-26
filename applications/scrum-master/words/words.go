// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package words runs scrum-master's commands, the words the Scrum Master profiles'
// exec declarations name, as one command line: from the rig's harness, or in
// the agent's own process when an agent build links this package in and
// serves Binary with Run (GH-27).
package words

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/commands"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/legacyenv"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/spectool"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// The words read SCRUM_MASTER_ variables, and the TPM_ names they had
// before GH-38 where a new one is unset; the package initializes before
// an agent build loads a profile, so its references see them too.
func init() {
	program.Words = Run
	legacyenv.Adopt()
}

// SetSpecTool runs the corpus gate's shape check with run, the spec tool's
// program an agent build linked in, instead of a tool built from a
// release's source (GH-33).
func SetSpecTool(run func(args []string, stdout, stderr io.Writer) int) { spectool.InProcess = run }

// Binary is the name the profiles' exec declarations give the words.
const Binary = program.WordsBinary

// Exit codes, as the words have always had them.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// Run runs one command line of words with dir as the working directory,
// the tracker routed from it, writing the output and any error to out, and
// returns the exit code. An empty dir is the current one. The working
// directory is the process's: a caller running words in its own process
// serializes calls and gets its directory back afterwards.
func Run(dir string, args []string, out io.Writer) int { return RunIn(dir, args, nil, out) }

// RunIn is Run with the command line's input, which a word such as
// scrum-master backlog guide reads (GH-59); nil is empty input.
func RunIn(dir string, args []string, in io.Reader, out io.Writer) int {
	if dir != "" {
		prev, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(out, "scrum-master:", err)
			return ExitError
		}
		if err := os.Chdir(dir); err != nil {
			fmt.Fprintln(out, "scrum-master:", err)
			return ExitError
		}
		defer os.Chdir(prev)
	}
	return mainIn(args, in, out, out)
}

// Main is the scrum-master command line from the current directory: the output to
// out, an error to errOut, and the exit code.
func Main(args []string, out, errOut io.Writer) int { return mainIn(args, os.Stdin, out, errOut) }

func mainIn(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Fprintln(out, "scrum-master", install.Version())
		return ExitOK
	}
	open := func() (tracker.Tracker, error) {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		return routing.Open(dir)
	}
	err := cli.Default.RunIn(args, in, out, open)
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, cli.ErrUsage):
		fmt.Fprintln(errOut, err)
		return ExitUsage
	default:
		fmt.Fprintln(errOut, "scrum-master:", err)
		return ExitError
	}
}
