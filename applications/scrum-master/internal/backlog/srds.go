// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/spectool"
)

// SRDDir holds the repository's software requirements documents.
const SRDDir = "docs/specs/software-requirements"

var (
	reqItem  = regexp.MustCompile(`(?m)^<!-- req: (R[0-9]+\.[0-9]+) -->$`)
	acStart  = regexp.MustCompile(`(?m)^<!-- ac: `)
	traceRef = regexp.MustCompile(`\[(R[0-9]+\.[0-9]+)\]`)
)

// checkSRDs reads the SRDs under dir and reports each requirement item no
// acceptance criterion traces, so a requirement nothing proves fails the
// gate, as a hierarchy item no criterion traces does (srd045 R6.1). It
// returns how many SRDs it read.
func checkSRDs(dir string) (int, []hierarchy.Finding, error) {
	paths, err := filepath.Glob(filepath.Join(dir, SRDDir, "srd*.md"))
	if err != nil {
		return 0, nil, err
	}
	var findings []hierarchy.Finding
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return 0, nil, err
		}
		text := string(b)
		criteria := ""
		if loc := acStart.FindStringIndex(text); loc != nil {
			criteria = text[loc[0]:]
		}
		var traced []string
		for _, m := range traceRef.FindAllStringSubmatch(criteria, -1) {
			traced = append(traced, m[1])
		}
		for _, m := range reqItem.FindAllStringSubmatch(text, -1) {
			if !slices.Contains(traced, m[1]) {
				findings = append(findings, hierarchy.Finding{File: p, Field: m[1], Message: "no acceptance criterion traces this requirement"})
			}
		}
	}
	return len(paths), findings, nil
}

// specCheck is the parent spec tool's check, replaced in tests.
var specCheck = spectool.Run

// notCanonical ends a spec tool line that only asks for formatting, which
// scrum-master doctor repairs; the gate reports the shape findings it cannot.
const notCanonical = ": not-canonical: run mage fmt"

// untracked names each document under docs/ that git does not track: the
// shape check reads only what git tracks, so such a document would pass
// unchecked until staged (srd045 R8.2). A directory git does not manage
// has none.
func untracked(dir string) []hierarchy.Finding {
	out, err := exec.Command("git", "-C", dir, "ls-files", "--others", "--exclude-standard", "--", "docs").Output()
	if err != nil {
		return nil
	}
	var findings []hierarchy.Finding
	for _, p := range strings.Fields(string(out)) {
		if strings.HasSuffix(p, ".md") {
			findings = append(findings, hierarchy.Finding{File: filepath.Join(dir, p), Field: "untracked",
				Message: "git does not track it, so the shape check cannot read it; git add it"})
		}
	}
	return findings
}

// withoutUntracked drops the documents untracked names, so a view built
// from what remains reads only what git tracks (GH-114).
func withoutUntracked(dir string, docs []*hierarchy.Doc) []*hierarchy.Doc {
	skip := map[string]bool{}
	for _, f := range untracked(dir) {
		if abs, err := filepath.Abs(f.File); err == nil {
			skip[abs] = true
		}
	}
	if len(skip) == 0 {
		return docs
	}
	return slices.DeleteFunc(slices.Clone(docs), func(d *hierarchy.Doc) bool {
		abs, err := filepath.Abs(d.Path)
		return err == nil && skip[abs]
	})
}

// unparsable matches a spec tool error that names a document and why it
// does not parse.
var unparsable = regexp.MustCompile(`(?:^|\n)(?:[^\n]*?: )?(docs/[^\s:]+\.md): ([^\n]+)`)

// checkShapes runs the pinned parent spec tool's check over the corpus at
// dir and reports every finding but formatting, so a document doctor
// would refuse, such as a VISION whose list field holds prose, fails the
// gate too (srd045 R6.1, srd043 R4.4). skipped says why it could not run:
// no pinned release, no Go toolchain, or no corpus document git tracks.
func checkShapes(dir string) (findings []hierarchy.Finding, skipped string) {
	lines, err := specCheck(dir, "check", "all")
	if err != nil && len(lines) == 0 {
		// A tool built before parse failures became findings aborts on the
		// first document it cannot read, naming it; that is a finding about
		// the document, not a reason the check could not run (srd045 R8.2).
		if m := unparsable.FindStringSubmatch(err.Error()); m != nil {
			return []hierarchy.Finding{{File: filepath.Join(dir, m[1]), Field: "parse", Message: emptyItemHint(m[2])}}, ""
		}
		return nil, err.Error()
	}
	for _, l := range lines {
		if strings.HasSuffix(l, notCanonical) {
			continue
		}
		file, rest, _ := strings.Cut(l, ": ")
		rule, message, ok := strings.Cut(rest, ": ")
		if !ok {
			rule, message = "spec", rest
		}
		findings = append(findings, hierarchy.Finding{File: filepath.Join(dir, file), Field: rule, Message: emptyItemHint(message)})
	}
	return findings, ""
}

// emptyItemHint adds to the format's refusal of a tree item with nothing
// under it, such as an empty cites, what to do about it (GH-63).
func emptyItemHint(message string) string {
	if strings.Contains(message, "an empty tree block") {
		return message + "; an item with nothing under it, such as an empty cites, is left out (design.md story)"
	}
	return message
}
