// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpusCiting builds the smallest corpus the check reads: use cases and their
// touchpoints.
func corpusCiting(root string, touchpoints map[string][]string) *Corpus {
	corpus := &Corpus{RootDir: root, UseCases: map[string]UseCase{}}
	for ucID, tps := range touchpoints {
		corpus.UCOrder = append(corpus.UCOrder, ucID)
		corpus.UseCases[ucID] = UseCase{Touchpoints: tps}
	}
	// UCOrder drives iteration, so sorting keeps the findings deterministic.
	for i := 1; i < len(corpus.UCOrder); i++ {
		for j := i; j > 0 && corpus.UCOrder[j] < corpus.UCOrder[j-1]; j-- {
			corpus.UCOrder[j], corpus.UCOrder[j-1] = corpus.UCOrder[j-1], corpus.UCOrder[j]
		}
	}
	return corpus
}

// writeBaseline writes the baseline document. Each line is "<use case> <srd>
// <group>", which the helper turns into the document's three fields, so a test
// reads as the citation it grandfathers rather than as YAML.
func writeBaseline(t *testing.T, root string, lines ...string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "id: test\ntitle: test\npurpose: test\ncitations:\n"
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("baseline line %q is not <use case> <srd> <group>", line)
		}
		body += fmt.Sprintf("  - {use_case: %s, srd: %s, group: %s}\n", fields[0], fields[1], fields[2])
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy-group-citations.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeRawBaseline writes the file verbatim, for the malformed cases.
func writeRawBaseline(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy-group-citations.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The GH-2339 guard. A group survives a rewrite unchanged, so a citation
// naming one keeps resolving after the requirement has come to mean something
// else; an item is renumbered or removed, which checkBrokenCitations reports.
func TestGroupLevelCitationsRequireAnItem(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	corpus := corpusCiting(root, map[string][]string{
		"rel13.0-uc001-collector-family": {
			"T3: srd020-collector R2 -- the receiver, await, spool, and stop words",
			"T4: srd020-collector R3.1 -- an item is precise enough to break loudly",
			"T5: srd020-collector AC8 -- an acceptance criterion is already an item",
		},
	})
	findings := checkGroupLevelCitations(corpus)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the bare R2", findings)
	}
	if findings[0].Level != "error" || findings[0].Check != "group-level-citation" {
		t.Errorf("finding = %+v, want an error from group-level-citation", findings[0])
	}
	for _, want := range []string{"rel13.0-uc001-collector-family", "srd020-collector", "R2", "R2.1"} {
		if !strings.Contains(findings[0].Message, want) {
			t.Errorf("message = %q, want it to name %q", findings[0].Message, want)
		}
	}
}

// The corpus carries hundreds of group citations that predate the rule, and
// retargeting one is a judgement rather than a rewrite. The baseline excuses
// exactly those and nothing else.
func TestGroupLevelCitationsHonourTheBaseline(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeBaseline(t, root, "rel13.0-uc001-collector-family srd020-collector R2")
	corpus := corpusCiting(root, map[string][]string{
		"rel13.0-uc001-collector-family": {"T3: srd020-collector R2 -- grandfathered"},
		"rel14.0-uc001-new-work":         {"T1: srd020-collector R2 -- written after the rule"},
	})
	findings := checkGroupLevelCitations(corpus)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want only the un-baselined citation", findings)
	}
	if !strings.Contains(findings[0].Message, "rel14.0-uc001-new-work") {
		t.Errorf("message = %q, want it to name the new use case", findings[0].Message)
	}
}

// A baseline that outlives what it excuses is a list nobody prunes, and the
// count it exists to hold down stops meaning anything.
func TestGroupLevelCitationsReportAStaleBaselineEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeBaseline(t, root, "rel13.0-uc001-collector-family srd020-collector R2")
	corpus := corpusCiting(root, map[string][]string{
		"rel13.0-uc001-collector-family": {"T3: srd020-collector R2.1 -- retargeted to an item"},
	})
	findings := checkGroupLevelCitations(corpus)
	if len(findings) != 1 || findings[0].Level != "warning" {
		t.Fatalf("findings = %+v, want one warning about the stale line", findings)
	}
	if !strings.Contains(findings[0].Message, "no use case cites") {
		t.Errorf("message = %q, want it to say the line is unused", findings[0].Message)
	}
}

// A corpus that never had a group citation owes no file to say so.
func TestGroupLevelCitationsTreatAnAbsentBaselineAsEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	corpus := corpusCiting(root, map[string][]string{
		"rel14.0-uc001-clean": {"T1: srd020-collector R2.1 -- an item"},
	})
	if findings := checkGroupLevelCitations(corpus); len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}

// A malformed baseline is reported rather than silently excusing nothing,
// which would turn every grandfathered citation into an error at once.
func TestGroupLevelCitationsRejectAMalformedBaseline(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// A citation missing its group: the document parses, but the entry cannot
	// name a citation, which is worse than a parse error because it would
	// silently excuse nothing.
	writeRawBaseline(t, root, "id: test\ntitle: test\npurpose: test\ncitations:\n  - {use_case: rel13.0-uc001-collector-family, srd: srd020-collector}\n")
	corpus := corpusCiting(root, map[string][]string{
		"rel13.0-uc001-collector-family": {"T3: srd020-collector R2 -- grandfathered"},
	})
	findings := checkGroupLevelCitations(corpus)
	if len(findings) != 1 || findings[0].Level != "error" {
		t.Fatalf("findings = %+v, want one error about the file", findings)
	}
	if !strings.Contains(findings[0].Message, legacyGroupCitationsFile) {
		t.Errorf("message = %q, want it to name the baseline file", findings[0].Message)
	}
}

// The shipped baseline has to match the shipped corpus: every line it carries
// is a citation that exists, and every group citation is a line it carries.
// Otherwise the repository's own audit is the only thing that notices.
func TestShippedBaselineMatchesTheShippedCorpus(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	entries, err := loadLegacyGroupCitations(root)
	if err != nil {
		t.Fatalf("load shipped baseline: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("shipped baseline is empty; the guard would be a no-op here")
	}
	for key := range entries {
		if fields := strings.Fields(key); len(fields) != 3 || !strings.HasPrefix(fields[1], "srd") {
			t.Errorf("baseline entry %q is not <use case> <srd> <group>", key)
		}
	}
}
