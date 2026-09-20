// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// A citation names a requirement group (R2) or an item inside one (R2.1).
// checkBrokenCitations catches a group that disappears; it cannot catch one
// that stays and comes to mean something else, which is the likelier outcome
// of an edit. GH-2329 rewrote srd020's R2 from "Catalog SRD" to "Surface
// separation" and a use case citing "srd020-collector R2" went on resolving
// while describing receiver words. Nothing failed.
//
// An item is the unit that does not survive a rewrite quietly: rewriting one
// usually renumbers or removes it, which checkBrokenCitations already reports.
// So a citation is required to name an item (GH-2339).
var groupOnlyCitation = regexp.MustCompile(`^R\d+$`)

// checkGroupLevelCitations reports a touchpoint citing a bare requirement
// group rather than an item within it.
//
// There is no grandfathering path, on purpose. The 328 citations that predated
// the rule were enumerated into their groups' items under GH-2390 — a
// claim-preserving rewrite, since citing a group and citing all of its items
// say the same thing — so a bare group here is always new, and a new one is
// written as items from the start.
func checkGroupLevelCitations(corpus *Corpus) []Finding {
	return groupLevelCitationFindings(groupCitations(corpus))
}

// groupCitations collects every citation naming a bare group, keyed
// "<use case> <srd> <group>" so the baseline can name one exactly.
func groupCitations(corpus *Corpus) []string {
	var cited []string
	seen := map[string]bool{}
	for _, ucID := range corpus.UCOrder {
		for _, tp := range corpus.UseCases[ucID].Touchpoints {
			srdID, groups := parseTouchpoint(tp)
			if srdID == "" {
				continue
			}
			for _, grp := range groups {
				if !groupOnlyCitation.MatchString(grp) {
					continue
				}
				key := ucID + " " + srdID + " " + grp
				if seen[key] {
					continue
				}
				seen[key] = true
				cited = append(cited, key)
			}
		}
	}
	return cited
}

// groupLevelCitationFindings turns each bare-group citation into an error.
func groupLevelCitationFindings(cited []string) []Finding {
	var findings []Finding
	for _, key := range cited {
		fields := strings.Fields(key)
		findings = append(findings, Finding{
			Check: "group-level-citation",
			Level: "error",
			Message: fmt.Sprintf(
				"use case %s cites %s %s, a requirement group; cite an item (%s.1) so a rewrite of the group cannot leave the citation resolving against a different meaning",
				fields[0], fields[1], fields[2], fields[2]),
		})
	}
	return findings
}
