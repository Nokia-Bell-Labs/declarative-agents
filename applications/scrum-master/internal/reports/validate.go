// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Finding is one report and what is wrong with it.
type Finding struct {
	Path     string   `json:"path"`
	Problems []string `json:"problems"`
}

// Validate checks the reports and rollups at paths, or every one under
// dir's status/ and a central repository's reports/ when paths is empty,
// and returns the ones that break their format (srd018 R2; GH-2725). A
// file under status/rollups/ is a rollup; every other one is a report.
func Validate(dir string, paths []string) (checked int, findings []Finding, err error) {
	if len(paths) == 0 {
		for _, root := range []string{filepath.Join(StatusDir, "reports"), filepath.Join(StatusDir, "rollups"), "reports"} {
			err = filepath.WalkDir(filepath.Join(dir, root), func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(p, ".md") {
					rel, _ := filepath.Rel(dir, p)
					paths = append(paths, rel)
				}
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				return 0, nil, err
			}
		}
	}
	for _, p := range paths {
		full := p
		if !filepath.IsAbs(p) {
			full = filepath.Join(dir, p)
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return checked, findings, err
		}
		checked++
		if problems := CheckFile(filepath.ToSlash(p), b); len(problems) > 0 {
			findings = append(findings, Finding{p, problems})
		}
	}
	return checked, findings, nil
}

// CheckFile checks one status file against its format, a rollup's or a
// report's by where it lives.
func CheckFile(p string, b []byte) []string {
	if IsRollupPath(p) {
		r, err := ParseRollup(p, b)
		if err != nil {
			return []string{err.Error()}
		}
		return r.Check()
	}
	r, err := ParseReport(p, b)
	if err != nil {
		return []string{err.Error()}
	}
	return r.Check()
}
