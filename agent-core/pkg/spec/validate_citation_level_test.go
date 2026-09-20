// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
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

func writeBaseline(t *testing.T, root string, lines ...string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# test baseline\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "legacy-group-citations.txt"), []byte(body), 0o644); err != nil {
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
	writeBaseline(t, root, "rel13.0-uc001-collector-family srd020-collector")
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
	for line := range entries {
		if fields := strings.Fields(line); len(fields) != 3 || !strings.HasPrefix(fields[1], "srd") {
			t.Errorf("baseline line %q is not <use case> <srd> <group>", line)
		}
	}
}
