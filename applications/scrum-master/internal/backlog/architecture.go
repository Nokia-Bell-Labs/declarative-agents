// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// architectureChecks compares the plan with the ARCHITECTURE diff since the
// latest tag (GH-56, srd045 R6.3): a component added or changed since the
// tag must be cited by an agreed hierarchy document. One that no document
// cites at all and no release shipped is drift, declared intent nobody
// planned; one cited only by documents not yet agreed is a coverage gap.
// Each component is named once. Everything is read locally: the working
// tree, the tag through git, and the policy from disk (srd045 R8.3).
func architectureChecks(dir string, docs []*hierarchy.Doc) (findings []hierarchy.Finding, refuse bool, err error) {
	_, now, err := workingComponents(dir)
	if err != nil || len(now) == 0 {
		return nil, false, err
	}
	deferred, err := deferredComponents(dir)
	if err != nil {
		return nil, false, err
	}
	tag := baselineTag(dir)
	shipped, err := taggedComponents(dir, tag)
	if err != nil {
		return nil, false, err
	}
	planned, cited := plannedMoves(dir, tag, docs)
	// Before any release there is no tag to measure from: nothing has
	// shipped, and every agreed document counts (GH-109).
	shippedBy, since, citedSince := "no release has shipped it yet", "", ""
	if tag != "" {
		shippedBy, since, citedSince = "no release up to "+tag+" shipped it", " since "+tag, " since then"
	}
	for _, id := range slices.Sorted(maps.Keys(now)) {
		before, wasShipped := shipped[id]
		// Declared deferred: intended, and planned by nobody on purpose,
		// whatever its text says now (GH-99).
		if deferred[id] || (wasShipped && before == now[id]) || planned[id] {
			continue
		}
		field := "components." + id
		switch {
		case !wasShipped && !cited[id]:
			findings = append(findings, hierarchy.Finding{File: hierarchy.ArchitectureFile, Field: field,
				Message: fmt.Sprintf("drift: %s and no hierarchy document cites it, so it is intent nobody planned or an architecture gone stale", shippedBy)})
		case !wasShipped:
			findings = append(findings, hierarchy.Finding{File: hierarchy.ArchitectureFile, Field: field,
				Message: fmt.Sprintf("uncovered: added%s, and no agreed document added or changed%s cites it", since, citedSince)})
		default:
			findings = append(findings, hierarchy.Finding{File: hierarchy.ArchitectureFile, Field: field,
				Message: fmt.Sprintf("uncovered: changed%s, and no agreed document added or changed%s cites it", since, citedSince)})
		}
	}
	findings = append(findings, planOnlyReleases(dir)...)
	if stale, err := statusPageStale(dir); err != nil {
		return nil, false, err
	} else if stale {
		findings = append(findings, hierarchy.Finding{File: StatusPage, Field: "generated",
			Message: "stale: the page sorts the components differently from what ARCHITECTURE, the latest release, and the hierarchy documents give now; regenerate it with the architecture-status skill"})
	}
	checks, err := architecturePolicy(dir)
	return findings, checks == program.ArchitectureRefuse, err
}

// statusPageStale reports whether the committed status page sorts the
// components into sections other than the view does now: a component
// missing, extra, or under another heading. Points and releases are left
// out, since they depend on the team checkouts beside this one, which CI
// lacks. A repository without the page has nothing stale.
func statusPageStale(dir string) (bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, StatusPage))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	// The page is current when it matches the view of every document on
	// disk, as the word renders it, or of the documents git tracks, as it
	// stood before one was written and not yet added (GH-114).
	page := pageSections(string(b))
	for _, trackedOnly := range []bool{false, true} {
		v, err := architecture(dir, trackedOnly, SiblingSources(dir), nil)
		if err != nil {
			return false, err
		}
		if maps.Equal(page, pageSections(v.Page())) {
			return false, nil
		}
	}
	return true, nil
}

// pageSections maps each component a status page lists to the heading it
// sits under.
func pageSections(page string) map[string]string {
	out, heading := map[string]string{}, ""
	for _, line := range strings.Split(page, "\n") {
		if h, ok := strings.CutPrefix(line, "## "); ok {
			heading = h
			continue
		}
		if rest, ok := strings.CutPrefix(line, "- **"); ok {
			if id, _, ok := strings.Cut(rest, "**"); ok {
				out[id] = heading
			}
		}
	}
	return out
}

// deferredComponents is the components whose status field ARCHITECTURE
// writes as deferred: intended, with no plan meant to move them yet, so no
// drift (GH-88).
func deferredComponents(dir string) (map[string]bool, error) {
	out := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(dir, hierarchy.ArchitectureFile))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	} else if err != nil {
		return nil, err
	}
	f, err := corpus.Decode(hierarchy.ArchitectureFile, data)
	if err != nil {
		return out, nil
	}
	for _, c := range f.Markdown.Records("component") {
		if s := c.Field("status"); s != nil && Deferred(s.Body) {
			out[c.Key] = true
		}
	}
	return out, nil
}

// Deferred reports whether a component's status says it is deferred: its
// text starts with the word, as in "Deferred: until the scale work".
func Deferred(status string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(status)), "deferred")
}

// workingComponents is the components of the working tree's ARCHITECTURE,
// in document order, with their texts.
func workingComponents(dir string) ([]string, map[string]string, error) {
	return componentTexts(func() ([]byte, error) { return os.ReadFile(filepath.Join(dir, hierarchy.ArchitectureFile)) })
}

// taggedComponents is the texts of the components ARCHITECTURE held at tag,
// none when tag is empty or the file did not exist there.
func taggedComponents(dir, tag string) (map[string]string, error) {
	if tag == "" {
		return map[string]string{}, nil
	}
	// Through textconv, so a repository that encrypts its documents at
	// rest (git-crypt) reads the tag's ARCHITECTURE as text (GH-88).
	_, texts, err := componentTexts(func() ([]byte, error) {
		return exec.Command("git", "-C", dir, "cat-file", "--textconv", tag+":./"+hierarchy.ArchitectureFile).Output()
	})
	return texts, err
}

// componentTexts reads an ARCHITECTURE document and maps each component id
// to its text, its heading and every body beneath it, so a change anywhere
// in a component counts, with the ids in document order; a document read
// returns no components when it is absent or does not parse, which the
// gate's own check reports.
func componentTexts(read func() ([]byte, error)) ([]string, map[string]string, error) {
	data, err := read()
	if err != nil {
		var exit *exec.ExitError
		if errors.Is(err, fs.ErrNotExist) || errors.As(err, &exit) {
			return nil, map[string]string{}, nil
		}
		return nil, nil, err
	}
	f, err := corpus.Decode(hierarchy.ArchitectureFile, data)
	if err != nil {
		return nil, map[string]string{}, nil
	}
	var ids []string
	out := map[string]string{}
	for _, c := range f.Markdown.Records("component") {
		ids = append(ids, c.Key)
		var b strings.Builder
		var walk func(*corpus.Section)
		walk = func(s *corpus.Section) {
			b.WriteString(s.Heading + "\n" + strings.TrimSpace(s.Body) + "\n")
			for _, child := range s.Children {
				walk(child)
			}
		}
		walk(c)
		out[c.Key] = b.String()
	}
	return ids, out, nil
}

// plannedMoves is the components an agreed document added or changed since
// the release tag cites, the plan for their move: an agreement older than
// the release planned an earlier move, not this one (GH-99); and every
// component any document cites.
func plannedMoves(dir, tag string, docs []*hierarchy.Doc) (planned, cited map[string]bool) {
	planned, cited = map[string]bool{}, map[string]bool{}
	for _, d := range docs {
		for _, c := range d.Architecture {
			cited[c] = true
			if d.Status == "agreed" && changedSince(dir, tag, d.Path) {
				planned[c] = true
			}
		}
	}
	return planned, cited
}

// ArchitectureMoves is the components of the working ARCHITECTURE added or
// changed since the latest released release, deferred ones aside, each
// mapped to whether an agreed document added or changed since then plans
// it (srd045 R6.3); plan write stops on a move only drafts plan (GH-100).
func ArchitectureMoves(dir string, docs []*hierarchy.Doc) (map[string]bool, error) {
	_, now, err := workingComponents(dir)
	if err != nil {
		return nil, err
	}
	deferred, err := deferredComponents(dir)
	if err != nil {
		return nil, err
	}
	tag := baselineTag(dir)
	shipped, err := taggedComponents(dir, tag)
	if err != nil {
		return nil, err
	}
	planned, _ := plannedMoves(dir, tag, docs)
	moves := map[string]bool{}
	for id, text := range now {
		if before, ok := shipped[id]; deferred[id] || (ok && before == text) {
			continue
		}
		moves[id] = planned[id]
	}
	return moves, nil
}

// baselineTag is the tag of the road map's latest released release, the
// release whose ARCHITECTURE the plan is compared with; empty while no
// release is released. A corpus tag also publishes each plan, so the
// latest tag is no release: taking it would empty the diff at every plan,
// and a branch cut before it would take an older run's (GH-99). The tag is
// read by name, so a worktree sees the same baseline as main.
func baselineTag(dir string) string {
	releases, err := roadMapReleases(dir)
	if err != nil {
		return ""
	}
	tag := ""
	for _, r := range releases {
		if r.status == "released" && r.tag != "" && !planOnly(dir, r) {
			if exec.Command("git", "-C", dir, "rev-parse", "--verify", "-q", r.tag+"^{commit}").Run() == nil {
				tag = r.tag
			}
		}
	}
	return tag
}

// planOnly reports whether a release the road map marks released names a
// tag that only published its plan: at that tag the release's record
// already names the tag, as plan write records it before the plan is
// tagged, and does not mark it released, so the tag holds the plan, not
// what the release shipped, and is no baseline (GH-117). A release tag cut
// before the road map named it is not one: the record there names the
// plan's tag. The road map is read through its textconv filter, as the
// tag's ARCHITECTURE is; a tag whose road map cannot be read is not judged.
func planOnly(dir string, r release) bool {
	b, err := exec.Command("git", "-C", dir, "cat-file", "--textconv", r.tag+":./docs/road-map.md").Output()
	if err != nil {
		return false
	}
	for _, then := range parseReleases(b) {
		if then.version == r.version {
			return then.tag == r.tag && then.status != "released"
		}
	}
	return false
}

// planOnlyReleases are the findings for each released release whose tag
// only published its plan (GH-117).
func planOnlyReleases(dir string) []hierarchy.Finding {
	releases, err := roadMapReleases(dir)
	if err != nil {
		return nil
	}
	var out []hierarchy.Finding
	for _, r := range releases {
		if r.status == "released" && r.tag != "" && planOnly(dir, r) {
			out = append(out, hierarchy.Finding{File: "docs/road-map.md", Field: "releases." + r.version + ".publication",
				Message: fmt.Sprintf("release %s is released but its tag %s only published its plan, so it is no baseline; name the release tag in its publication", r.version, r.tag)})
		}
	}
	return out
}

// changedSince reports whether the document at path was added or changed
// since tag; every document is, before any release.
func changedSince(dir, tag, path string) bool {
	if tag == "" {
		return true
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return true
	}
	// A document loaded from a relative dir has a relative path, which
	// Rel cannot take against an absolute root (GH-114).
	if path, err = filepath.Abs(path); err != nil {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	rel = filepath.ToSlash(rel)
	if exec.Command("git", "-C", dir, "cat-file", "-e", tag+":"+rel).Run() != nil {
		return true
	}
	return exec.Command("git", "-C", dir, "diff", "--quiet", tag, "--", rel).Run() != nil
}

// policyData is the policy document on disk: the repository's own
// control/policy.md, else the system repository's beside it; nil when
// neither exists.
func policyData(dir string) ([]byte, error) {
	paths := []string{filepath.Join(dir, program.PolicyFile)}
	if trackers, err := checkout.FindTrackers(dir); err == nil && trackers != "" {
		paths = append(paths, filepath.Join(filepath.Dir(filepath.Dir(trackers)), program.PolicyFile))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return data, err
	}
	return nil, nil
}

// architecturePolicy is the program's architecture_checks, from the
// policy on disk: the repository's own control/policy.md, else the system
// repository's beside it, else the default.
func architecturePolicy(dir string) (string, error) {
	data, err := policyData(dir)
	if err != nil || data == nil {
		return program.ArchitectureWarn, err
	}
	checks, err := program.ArchitectureChecksIn(data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", program.PolicyFile, err)
	}
	return checks, nil
}
