// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// legacyGroupCitationsFile lists the citations that predate this rule, one
// "<use case> <srd> <group>" per line.
//
// The corpus carries 538 of them across 72 use cases, and retargeting one is
// a judgement about which item its prose meant rather than a rewrite, so they
// are grandfathered the way declstyle grandfathers pre-type-system
// declarations. The baseline stops the count growing; it does not shrink it.
// An entry that no longer matches any citation is reported, so the file cannot
// outlive what it excuses.
const legacyGroupCitationsFile = "docs/specs/legacy-group-citations.txt"

// checkGroupLevelCitations reports a touchpoint citing a bare requirement
// group rather than an item within it, and a baseline line that no longer
// excuses anything.
func checkGroupLevelCitations(corpus *Corpus) []Finding {
	baseline, err := loadLegacyGroupCitations(corpus.RootDir)
	if err != nil {
		return []Finding{{
			Check:   "group-level-citation",
			Level:   "error",
			Message: fmt.Sprintf("cannot read %s: %v", legacyGroupCitationsFile, err),
		}}
	}
	cited := groupCitations(corpus)
	findings := unbaselinedGroupCitations(cited, baseline)
	return append(findings, staleGroupCitationBaselineEntries(cited, baseline)...)
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

// unbaselinedGroupCitations reports the citations the baseline does not excuse.
func unbaselinedGroupCitations(cited []string, baseline map[string]bool) []Finding {
	var findings []Finding
	for _, key := range cited {
		if baseline[key] {
			continue
		}
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

// staleBaselineEntries reports a grandfathered citation nothing writes any
// more, so the file cannot outlive what it excuses.
func staleGroupCitationBaselineEntries(cited []string, baseline map[string]bool) []Finding {
	present := make(map[string]bool, len(cited))
	for _, key := range cited {
		present[key] = true
	}
	var stale []string
	for key := range baseline {
		if !present[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	findings := make([]Finding, 0, len(stale))
	for _, key := range stale {
		findings = append(findings, Finding{
			Check: "group-level-citation",
			Level: "warning",
			Message: fmt.Sprintf(
				"%s grandfathers %q, which no use case cites any more; delete the line",
				legacyGroupCitationsFile, key),
		})
	}
	return findings
}

// loadLegacyGroupCitations reads the baseline. An absent file is an empty
// baseline rather than an error: a corpus that never had a group citation
// needs no file to say so.
func loadLegacyGroupCitations(rootDir string) (map[string]bool, error) {
	path := filepath.Join(rootDir, filepath.FromSlash(legacyGroupCitationsFile))
	file, err := os.Open(path) // #nosec G304 -- corpus-relative path owned by the repository
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()

	entries := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if fields := strings.Fields(line); len(fields) == 3 {
			entries[strings.Join(fields, " ")] = true
			continue
		}
		return nil, fmt.Errorf("line %q is not <use case> <srd> <group>", line)
	}
	return entries, scanner.Err()
}
