// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/spectool"
)

// outOfTreeModule is the module path the copy is renamed to: no code may
// depend on the application's own.
const outOfTreeModule = "example.invalid/scrum-master"

// OutOfTree runs the audit, the unit tests, and the cross build on a copy
// of the application outside this repository, its Go module renamed to a
// placeholder and its runtime the agent-core this repository holds, named
// in the copy's docs/external-checkouts.yaml as a parent checkout (GH-2754).
// A gate or a test that depends on the repository's layout or on the
// application's module path fails it. Outside the repository that ships
// agent-core there is nothing to compare with, and it says so.
func OutOfTree() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	rel, ok, err := runtime.Sibling(root, nil)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("outOfTree: the application is already outside the repository that ships agent-core")
		return nil
	}
	tmp, err := os.MkdirTemp("", "scrum-master-out-of-tree-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	dst := filepath.Join(tmp, "scrum-master")
	if err := copyApplication(root, dst); err != nil {
		return err
	}
	if err := renameModule(dst, outOfTreeModule); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dst, spectool.CheckoutsFile),
		[]byte(fmt.Sprintf("# Written by mage outOfTree: the parent is this repository's checkout.\ncheckouts:\n  %s: %s\n", spectool.ParentRepository, rel.Src)), 0o644); err != nil {
		return err
	}
	env := append(os.Environ(), "GOWORK=off")
	for _, step := range [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "add", "-A"},
		{"git", "-c", "user.name=out-of-tree", "-c", "user.email=out-of-tree@example.invalid", "commit", "-q", "-m", "the application, out of tree"},
		{"mage", "audit"},
		{"go", "test", "./...", "-count=1"},
		{"mage", "crossBuild"},
	} {
		cmd := exec.Command(step[0], step[1:]...)
		cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = dst, env, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("out of tree, %s: %w", strings.Join(step, " "), err)
		}
	}
	return nil
}

// copyApplication copies the files git tracks, and those it would add, of
// the application at root into dst.
func copyApplication(root, dst string) error {
	out, err := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard", "-z", ".").Output()
	if err != nil {
		return fmt.Errorf("list the application's files: %w", err)
	}
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" {
			continue
		}
		src := filepath.Join(root, rel)
		info, err := os.Lstat(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, b, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

// renameModule renames the Go module at dir, rewriting every Go file's
// imports of it.
func renameModule(dir, module string) error {
	gomod := filepath.Join(dir, "go.mod")
	b, err := os.ReadFile(gomod)
	if err != nil {
		return err
	}
	first, _, _ := strings.Cut(string(b), "\n")
	old := strings.TrimSpace(strings.TrimPrefix(first, "module"))
	if old == "" {
		return fmt.Errorf("%s names no module", gomod)
	}
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || (filepath.Ext(p) != ".go" && p != gomod) {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if s := string(b); strings.Contains(s, old) {
			if err := os.WriteFile(p, []byte(strings.ReplaceAll(s, old, module)), 0o644); err != nil {
				return err
			}
			// Imports sort by path, so a renamed one moves, as any rename
			// tool would format it.
			if filepath.Ext(p) == ".go" {
				return exec.Command("gofmt", "-w", p).Run()
			}
		}
		return nil
	})
}
