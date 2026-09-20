// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
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

// The corpus GH-2390 left behind: every citation is an item list. The check
// has nothing to say about it, and there is no baseline to consult.
func TestEnumeratedItemCitationsPassWithoutABaseline(t *testing.T) {
	t.Parallel()
	corpus := corpusCiting(t.TempDir(), map[string][]string{
		"rel00.0-uc001-domain-agent-integration": {
			"T1: srd001-core-types R1.1, R1.2, R1.3, R1.4, R1.5, R2.1, R2.2, R2.3 -- the enumerated form",
		},
	})
	if findings := checkGroupLevelCitations(corpus); len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}
