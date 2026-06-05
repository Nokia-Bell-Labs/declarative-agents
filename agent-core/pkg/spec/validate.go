// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"sort"
	"strings"
)

// Finding represents a single validation result.
type Finding struct {
	Check   string // short check identifier
	Level   string // "error" or "warning"
	Message string
}

// Validate runs all consistency checks on the graph and returns findings.
func Validate(g *Graph, corpus *Corpus) []Finding {
	var all []Finding
	all = append(all, checkOrphanedSRDs(g)...)
	all = append(all, checkBrokenTouchpoints(g)...)
	all = append(all, checkBrokenCitations(g, corpus)...)
	all = append(all, checkBareTouchpoints(g, corpus)...)
	all = append(all, checkOrphanedTestSuites(g)...)
	all = append(all, checkUncoveredReqItems(g)...)
	all = append(all, checkUncoveredACs(g)...)
	all = append(all, checkUntracedSuccessCriteria(g, corpus)...)
	all = append(all, checkDependsOnViolations(g)...)
	all = append(all, checkReleasesWithoutTestSuites(g, corpus)...)
	return all
}

// checkOrphanedSRDs finds SRDs that no use case touches.
func checkOrphanedSRDs(g *Graph) []Finding {
	var findings []Finding
	for _, srd := range g.NodesByKind(KindSRD) {
		incoming := g.IncomingByRel(srd.ID, RelTouches)
		if len(incoming) == 0 {
			findings = append(findings, Finding{
				Check:   "orphaned-srd",
				Level:   "warning",
				Message: fmt.Sprintf("SRD %s is not referenced by any use case touchpoint", srd.ID),
			})
		}
	}
	return findings
}

// checkBrokenTouchpoints verifies that use case touches/cites edges point
// to nodes that actually exist. Since BuildGraph only creates edges to
// existing nodes, we check by comparing the touchpoint SRD references
// in the corpus against the SRD nodes in the graph.
func checkBrokenTouchpoints(g *Graph) []Finding {
	var findings []Finding
	for _, uc := range g.NodesByKind(KindUseCase) {
		touches := g.OutgoingByRel(uc.ID, RelTouches)
		for _, targetID := range touches {
			if _, ok := g.Node(targetID); !ok {
				findings = append(findings, Finding{
					Check:   "broken-touchpoint",
					Level:   "error",
					Message: fmt.Sprintf("use case %s touchpoint references non-existent SRD %s", uc.ID, targetID),
				})
			}
		}
	}
	return findings
}

// checkBrokenCitations verifies that cites edges from use cases to
// requirement groups reference groups that exist in the graph.
func checkBrokenCitations(g *Graph, corpus *Corpus) []Finding {
	var findings []Finding
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		for _, tp := range uc.Touchpoints {
			srdID, groups := parseTouchpoint(tp)
			if srdID == "" {
				continue
			}
			for _, grp := range groups {
				groupNodeID := srdID + ":" + grp
				if _, ok := g.Node(groupNodeID); !ok {
					findings = append(findings, Finding{
						Check:   "broken-citation",
						Level:   "error",
						Message: fmt.Sprintf("use case %s cites %s %s but requirement group not found", ucID, srdID, grp),
					})
				}
			}
		}
	}
	return findings
}

// checkBareTouchpoints flags use case touchpoints that cite an SRD
// without specifying any requirement group references.
func checkBareTouchpoints(g *Graph, corpus *Corpus) []Finding {
	var findings []Finding
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		for _, tp := range uc.Touchpoints {
			srdID, groups := parseTouchpoint(tp)
			if srdID == "" {
				continue
			}
			if len(groups) == 0 {
				findings = append(findings, Finding{
					Check:   "bare-touchpoint",
					Level:   "warning",
					Message: fmt.Sprintf("use case %s cites %s without R-group references", ucID, srdID),
				})
			}
		}
	}
	return findings
}

// checkOrphanedTestSuites finds test suites whose covers edges
// don't connect to any use case node that exists in the graph.
func checkOrphanedTestSuites(g *Graph) []Finding {
	var findings []Finding
	for _, ts := range g.NodesByKind(KindTestSuite) {
		covers := g.OutgoingByRel(ts.ID, RelCovers)
		hasUC := false
		for _, targetID := range covers {
			if n, ok := g.Node(targetID); ok && n.Kind == KindUseCase {
				hasUC = true
				break
			}
		}
		if !hasUC {
			findings = append(findings, Finding{
				Check:   "orphaned-test-suite",
				Level:   "warning",
				Message: fmt.Sprintf("test suite %s traces don't reference any known use case", ts.ID),
			})
		}
	}
	return findings
}

// checkUncoveredReqItems finds requirement items that are not traced
// by any acceptance criterion.
func checkUncoveredReqItems(g *Graph) []Finding {
	var findings []Finding
	for _, item := range g.NodesByKind(KindReqItem) {
		incoming := g.IncomingByRel(item.ID, RelTraces)
		if len(incoming) == 0 {
			findings = append(findings, Finding{
				Check:   "uncovered-req-item",
				Level:   "error",
				Message: fmt.Sprintf("requirement item %s not covered by any acceptance criterion", item.ID),
			})
		}
	}
	return findings
}

// checkUncoveredACs finds acceptance criteria not covered by any test case.
func checkUncoveredACs(g *Graph) []Finding {
	var findings []Finding
	for _, ac := range g.NodesByKind(KindAC) {
		incoming := g.IncomingByRel(ac.ID, RelCovers)
		if len(incoming) == 0 {
			findings = append(findings, Finding{
				Check:   "uncovered-ac",
				Level:   "warning",
				Message: fmt.Sprintf("acceptance criterion %s not covered by any test case", ac.ID),
			})
		}
	}
	return findings
}

// checkUntracedSuccessCriteria finds use case success criteria that
// don't cite any AC in their traces.
func checkUntracedSuccessCriteria(g *Graph, corpus *Corpus) []Finding {
	var findings []Finding
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		for _, sc := range uc.SuccessCriteria {
			hasACTrace := false
			for _, tr := range sc.Traces {
				parts := strings.Fields(tr)
				if len(parts) >= 2 && strings.HasPrefix(parts[0], "srd") && strings.HasPrefix(parts[1], "AC") {
					hasACTrace = true
					break
				}
			}
			if !hasACTrace {
				findings = append(findings, Finding{
					Check:   "untraced-success-criterion",
					Level:   "warning",
					Message: fmt.Sprintf("use case %s success criterion %s has no AC trace", ucID, sc.ID),
				})
			}
		}
	}
	return findings
}

// checkDependsOnViolations verifies that inter-SRD depends_on references
// point to SRDs that exist.
func checkDependsOnViolations(g *Graph) []Finding {
	var findings []Finding
	for _, srd := range g.NodesByKind(KindSRD) {
		deps := g.OutgoingByRel(srd.ID, RelDependsOn)
		for _, depID := range deps {
			if _, ok := g.Node(depID); !ok {
				findings = append(findings, Finding{
					Check:   "depends-on-violation",
					Level:   "error",
					Message: fmt.Sprintf("SRD %s depends_on %s which does not exist", srd.ID, depID),
				})
			}
		}
	}
	return findings
}

// checkReleasesWithoutTestSuites verifies that each release with use cases
// has a corresponding test suite.
func checkReleasesWithoutTestSuites(g *Graph, corpus *Corpus) []Finding {
	var findings []Finding

	testSuiteReleases := make(map[string]bool)
	for _, ts := range g.NodesByKind(KindTestSuite) {
		if ts.Release != "" {
			testSuiteReleases[ts.Release] = true
		}
	}

	for _, rel := range g.NodesByKind(KindRelease) {
		version := rel.Release
		if version == "" {
			continue
		}
		hasUCs := false
		for _, r := range corpus.Roadmap.Releases {
			if r.Version == version && len(r.UseCases) > 0 {
				hasUCs = true
				break
			}
		}
		if hasUCs && !testSuiteReleases[version] {
			findings = append(findings, Finding{
				Check:   "release-without-test-suite",
				Level:   "warning",
				Message: fmt.Sprintf("release %s has use cases but no test suite", version),
			})
		}
	}
	return findings
}

// Errors returns only error-level findings.
func Errors(findings []Finding) []Finding {
	var errs []Finding
	for _, f := range findings {
		if f.Level == "error" {
			errs = append(errs, f)
		}
	}
	return errs
}

// Warnings returns only warning-level findings.
func Warnings(findings []Finding) []Finding {
	var warns []Finding
	for _, f := range findings {
		if f.Level == "warning" {
			warns = append(warns, f)
		}
	}
	return warns
}

// FormatFindings produces a sorted human-readable report.
func FormatFindings(findings []Finding) string {
	if len(findings) == 0 {
		return "All consistency checks passed.\n"
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Level != findings[j].Level {
			return findings[i].Level == "error"
		}
		if findings[i].Check != findings[j].Check {
			return findings[i].Check < findings[j].Check
		}
		return findings[i].Message < findings[j].Message
	})

	var b strings.Builder
	currentCheck := ""
	for _, f := range findings {
		if f.Check != currentCheck {
			currentCheck = f.Check
			fmt.Fprintf(&b, "\n[%s] %s:\n", f.Level, f.Check)
		}
		fmt.Fprintf(&b, "  - %s\n", f.Message)
	}
	return b.String()
}
