// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Every document type that declares a references field gets it checked for
// presence. Nothing resolved the entries, so a path that moved stayed in the
// list and read as current: eng02 cited a rest/definition.go that had been
// split into server_static_assets.go, and no gate saw it (GH-2341).
//
// The entries are prose, not a schema. These are the shapes the corpus uses:
//
//	magefiles/kindrig/diagnose.go (shared on-demand diagnosis)
//	applications/.../srd020-collector.yaml R7.4 (collector static_assets root)
//	kind Quick Start, https://kind.sigs.k8s.io/docs/user/quick-start/
//	GH-1316 (chatbot-mesh ux/ to agents/chatbot/ui migration)
//	agent-core/docs/specs/software-requirements/srd013 R5.6/R5.7 (parameterization)
//
// Only the first two name a file. The last one is the trap: it cites an SRD by
// id, and the file on disk is srd013-standard-tool-library.yaml, so reading it
// as a path reports a miss that is really a citation style.
var (
	referenceIssuePattern = regexp.MustCompile(`^GH-\d+`)
	referenceExtPattern   = regexp.MustCompile(`\.[A-Za-z0-9]+$`)
)

// referencePath extracts the repo-relative path an entry names, or "" when the
// entry names something other than a file.
func referencePath(entry string) string {
	candidate := strings.TrimSpace(entry)
	if candidate == "" {
		return ""
	}
	// A note in parentheses describes the reference; it never holds the path.
	if index := strings.Index(candidate, " ("); index >= 0 {
		candidate = candidate[:index]
	}
	// A title-and-URL entry is separated by a comma, and the URL is not ours.
	if strings.Contains(candidate, ", http") || strings.Contains(entry, ", http") {
		return ""
	}
	// The path is the first token; a trailing requirement id such as R7.4
	// qualifies the reference rather than extending the filename.
	if fields := strings.Fields(candidate); len(fields) != 0 {
		candidate = fields[0]
	}
	switch {
	case candidate == "":
		return ""
	case strings.HasPrefix(candidate, "http://"), strings.HasPrefix(candidate, "https://"):
		return ""
	case referenceIssuePattern.MatchString(candidate):
		return ""
	case !strings.Contains(candidate, "/"):
		return ""
	case !referenceExtPattern.MatchString(candidate):
		// An id-shaped citation such as .../srd013 names a document by id, not
		// by filename. Resolving it would report a miss that is not one.
		return ""
	}
	return candidate
}

// referenceBases lists the directories an entry may be written against, from
// the repository root down to the document's own directory.
//
// Two conventions are in use and both are legitimate. A repository-level
// document writes repo-relative paths (docs/engineering/eng01 cites
// magefiles/kindrig/diagnose.go). A module's own document writes them relative
// to its module (agent-core/docs/constitutions/design.yaml cites
// docs/VISION.yaml, meaning agent-core/docs/VISION.yaml). Resolving against
// every ancestor accepts both without naming the modules, so a module added
// later needs no change here.
func referenceBases(candidate string) []string {
	bases := []string{""}
	parts := strings.Split(filepath.ToSlash(filepath.Dir(candidate)), "/")
	for index := range parts {
		if parts[index] == "." || parts[index] == "" {
			continue
		}
		bases = append(bases, strings.Join(parts[:index+1], "/"))
	}
	return bases
}

// referenceResolves reports whether an entry's path exists under any base the
// document could have written it against.
func referenceResolves(root, candidate, path string) bool {
	for _, base := range referenceBases(candidate) {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(base), filepath.FromSlash(path))); err == nil {
			return true
		}
	}
	return false
}

// documentReferenceFailures reports every references entry in one document
// that names a path which does not exist under any base it could mean.
func documentReferenceFailures(root, candidate string) []string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(candidate)))
	if err != nil {
		return []string{fmt.Sprintf("%s: cannot be read: %v", candidate, err)}
	}
	var document struct {
		References []string `yaml:"references"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		// Field presence and parseability are already reported by the
		// placement check; a references list this shape cannot hold is its
		// finding to make, not this one's.
		return nil
	}
	var failures []string
	for _, entry := range document.References {
		path := referencePath(entry)
		if path == "" {
			continue
		}
		if !referenceResolves(root, candidate, path) {
			failures = append(failures, fmt.Sprintf(
				"%s: references entry names %q, which does not exist", candidate, path))
		}
	}
	return failures
}
