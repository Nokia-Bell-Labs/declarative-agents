// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// discardingTestRun matches a mage target that runs a test suite through
// sh.Run. sh.Run sends the command's stdout to a nil writer unless mage runs
// verbose (sh.RunWith consults mg.Verbose), and go test writes its failures to
// stdout, so the suite's output is discarded and the lane reports an exit code
// with no test named. sh.RunV always streams.
var discardingTestRun = regexp.MustCompile(`sh\.Run\(\s*"(?:go|mage)"\s*,\s*"test`)

// TestTestRunnersStreamTheirOutput keeps a failing lane able to name what
// failed. GH-2055: mage test reported `tests in agent-core: exit status 1`
// with no --- FAIL line anywhere, because agent-core's own test target
// discarded the go test output it had just produced.
func TestTestRunnersStreamTheirOutput(t *testing.T) {
	t.Parallel()
	offenders, err := discardingTestRunners(repositoryRootForOutput(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("test runners discard their output; use sh.RunV so a failing "+
			"lane names the test:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// TestDiscardingTestRunIsDetected is the negative half: the matcher fires on
// the form the repository used before GH-2055 and not on the form it uses now.
func TestDiscardingTestRunIsDetected(t *testing.T) {
	t.Parallel()
	for _, discarded := range []string{
		`return sh.Run("go", "test", "-short", "-timeout", "5m", "./...")`,
		`sh.Run("mage", "test")`,
		`	if err := sh.Run( "go" , "test" ); err != nil {`,
	} {
		if !discardingTestRun.MatchString(discarded) {
			t.Errorf("did not detect a discarded test run: %s", discarded)
		}
	}
	for _, streamed := range []string{
		`return sh.RunV("go", "test", "-short", "./...")`,
		`sh.Run("go", "build", "./...")`,
		`sh.Run("helm", "test", "release")`,
	} {
		if discardingTestRun.MatchString(streamed) {
			t.Errorf("flagged a run that does not discard test output: %s", streamed)
		}
	}
}

func discardingTestRunners(root string) ([]string, error) {
	var offenders []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if skipForOutputScan(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for number, line := range strings.Split(string(data), "\n") {
			if !discardingTestRun.MatchString(line) {
				continue
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			offenders = append(offenders,
				fmt.Sprintf("%s:%d", filepath.ToSlash(relative), number+1))
		}
		return nil
	})
	sort.Strings(offenders)
	return offenders, err
}

func skipForOutputScan(name string) bool {
	switch name {
	case ".git", "build", "generated-files", "node_modules", "testdata", "vendor":
		return true
	}
	return false
}

func repositoryRootForOutput(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
