// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// LineageFile describes a program and its place among programs (config
// format program): its own site and, when it is a team of a larger
// program, that parent (GH-2740).
const LineageFile = "control/program.md"

// Lineage is a program's control/program.md. Every field is optional.
type Lineage struct {
	// Site is the program's site URL; SiteRepository the repository
	// holding it, its reports repository, as a project path or URL.
	Site           string `yaml:"site,omitempty"`
	SiteRepository string `yaml:"site_repository,omitempty"`
	// Parent is the parent program's system repository and ParentSite
	// its site URL.
	Parent     string `yaml:"parent,omitempty"`
	ParentSite string `yaml:"parent_site,omitempty"`
}

// LineageIn reads the lineage of the system checkout at dir; a program
// without control/program.md has none.
func LineageIn(dir string) (Lineage, error) {
	b, err := os.ReadFile(filepath.Join(dir, LineageFile))
	if errors.Is(err, os.ErrNotExist) {
		return Lineage{}, nil
	}
	if err != nil {
		return Lineage{}, err
	}
	return ParseLineage(b)
}

// LoadLineage reads a system repository's lineage at ref through the
// tracker; a program without control/program.md has none.
func LoadLineage(t tracker.Forge, system, ref string) (Lineage, error) {
	b, err := t.GetFile(system, LineageFile, ref)
	if errors.Is(err, tracker.ErrNotFound) {
		return Lineage{}, nil
	}
	if err != nil {
		return Lineage{}, err
	}
	return ParseLineage(b)
}

// ParseLineage reads a control/program.md.
func ParseLineage(data []byte) (Lineage, error) {
	var l Lineage
	if err := corpus.DecodeConfig(data, &l); err != nil {
		return l, fmt.Errorf("%s: %w", LineageFile, err)
	}
	for _, v := range []*string{&l.Site, &l.SiteRepository, &l.Parent, &l.ParentSite} {
		*v = strings.TrimSpace(*v)
	}
	return l, nil
}

// SitePage is the URL of a site's page for a site path such as
// hierarchy/ep001-x.md: the shell's route on the site at base.
func SitePage(base, page string) string {
	if base == "" {
		return ""
	}
	return strings.TrimSuffix(base, "/") + "/#/" + strings.TrimSuffix(strings.TrimPrefix(page, "/"), ".md")
}
