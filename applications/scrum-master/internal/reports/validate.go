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

// Validate checks the reports at paths, or every report under dir's
// reports/ when paths is empty, and returns the ones that break the format
// (srd018 R2).
func Validate(dir string, paths []string) (checked int, findings []Finding, err error) {
	if len(paths) == 0 {
		err = filepath.WalkDir(filepath.Join(dir, "reports"), func(p string, d fs.DirEntry, err error) error {
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
		r, err := ParseReport(filepath.ToSlash(p), b)
		if err != nil {
			findings = append(findings, Finding{p, []string{err.Error()}})
			continue
		}
		if problems := r.Check(); len(problems) > 0 {
			findings = append(findings, Finding{p, problems})
		}
	}
	return checked, findings, nil
}
