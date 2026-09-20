// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
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

// legacyGroupCitationsFile lists the citations that predate this rule.
//
// The corpus carries 538 of them across 72 use cases, and retargeting one is
// a judgement about which item its prose meant rather than a rewrite, so they
// are grandfathered the way declstyle grandfathers pre-type-system
// declarations. The baseline stops the count growing; it does not shrink it.
// An entry that no longer matches any citation is reported, so the file cannot
// outlive what it excuses.
const legacyGroupCitationsFile = "docs/specs/legacy-group-citations.yaml"

// legacyGroupCitations is the baseline document. It is YAML and declared in
// the document-type registry rather than a bare list, because every tracked
// file under a docs path has to name a document type: a text baseline there
// matched none, and main went red when the placement check and the baseline
// landed in the same week (GH-2372).
type legacyGroupCitations struct {
	Citations []legacyGroupCitation `yaml:"citations"`
}

// legacyGroupCitation is one grandfathered citation.
type legacyGroupCitation struct {
	UseCase string `yaml:"use_case"`
	SRD     string `yaml:"srd"`
	Group   string `yaml:"group"`
}

// key is the form the check compares against, so the document's three fields
// and the check's lookup cannot drift apart.
func (c legacyGroupCitation) key() string {
	return c.UseCase + " " + c.SRD + " " + c.Group
}

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
	data, err := os.ReadFile(path) // #nosec G304 -- corpus-relative path owned by the repository
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	var document legacyGroupCitations
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	entries := make(map[string]bool, len(document.Citations))
	for _, citation := range document.Citations {
		if citation.UseCase == "" || citation.SRD == "" || citation.Group == "" {
			return nil, fmt.Errorf("citation %+v needs use_case, srd, and group", citation)
		}
		entries[citation.key()] = true
	}
	return entries, nil
}
