// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
)

// moveRoster moves a program's roster from the central reports repository
// checked out beside the system checkout at dir into the system
// repository's control/reports.md, naming the central repository so collect
// and previous still read its reports for the release the program moves in
// (srd018 R12.2; GH-2725). A system repository that already holds a roster,
// or has no central reports checkout beside it, is left as it is.
func moveRoster(dir string, dryRun bool) (Result, error) {
	target := filepath.Join(dir, filepath.FromSlash(reports.ControlFile))
	if _, err := os.Stat(target); err == nil {
		return Result{}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Result{}, err
	}
	for _, sib := range checkout.Siblings(dir) {
		cfg, err := reports.LoadConfig(sib)
		if err != nil {
			continue
		}
		cfg.Reports = repositoryOf(sib)
		cfg.System = ""
		doc, err := reports.RosterDocument(cfg)
		if err != nil {
			return Result{}, err
		}
		r := Result{Path: reports.ControlFile, Action: "moved",
			Reason: "the roster from " + filepath.Base(sib) + "/" + reports.ConfigFile + "; collect still reads " + cfg.Reports + "'s reports for this release"}
		if dryRun {
			return r, nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return Result{}, err
		}
		return r, os.WriteFile(target, doc, 0o644)
	}
	return Result{}, nil
}

// repositoryOf names a checkout's repository by its origin, else by its
// folder.
func repositoryOf(dir string) string {
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if origin := strings.TrimSpace(string(out)); err == nil && origin != "" && !strings.HasPrefix(origin, "/") {
		if p := program.ProjectPath(origin); p != "" {
			return p
		}
	}
	return filepath.Base(checkout.MainCheckout(dir))
}
