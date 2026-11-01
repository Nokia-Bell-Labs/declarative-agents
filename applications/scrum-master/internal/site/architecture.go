// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// ArchitectureDir is the architecture word's folder: a page per subsystem
// and per component, a component's below its subsystem (srd048 R9.1).
const ArchitectureDir = "architecture"

// ArchitecturePages are the architecture word's pages, drawn from the
// timeline facts the timeline word wrote into the reports checkout dir:
// the folder's index as the tree, a page per subsystem with its owner, its
// contract, and its scrum, and a page per component with its lifecycle
// state, its contract, and the planning documents citing it (srd048 R9.1).
func ArchitecturePages(dir string) (map[string][]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, Dir, "timeline", "facts.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s holds no timeline facts; run the site's timeline word first", dir)
	} else if err != nil {
		return nil, err
	}
	var f timeline.Facts
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s/timeline/facts.json: %w", Dir, err)
	}
	corpus := func(p string) bool {
		_, err := os.Stat(filepath.Join(dir, Dir, CorpusDir, filepath.FromSlash(p)))
		return err == nil
	}
	pages := map[string][]byte{}
	pages[Dir+"/"+ArchitectureDir+"/index.md"] = []byte(architectureIndex(f, corpus))
	for _, sub := range f.Architecture {
		pages[Dir+"/"+subsystemPath(sub)] = []byte(subsystemPage(sub, corpus))
		for _, c := range sub.Components {
			pages[Dir+"/"+componentPath(sub, c)] = []byte(componentPage(sub, c, f))
		}
	}
	return pages, nil
}

// subsystemPath is a subsystem's page under site/, componentPath a
// component's, below its subsystem (R9.1).
func subsystemPath(sub timeline.SubsystemFact) string {
	return ArchitectureDir + "/" + sub.ID + ".md"
}

func componentPath(sub timeline.SubsystemFact, c timeline.ComponentFact) string {
	return ArchitectureDir + "/" + sub.ID + "/" + c.ID + ".md"
}

// ArchPages maps each component, keyed as labels compare, to its page
// under site/: what the hierarchy, scrum, and navigator links resolve a
// citation against (R9.2).
func ArchPages(arch []timeline.SubsystemFact) map[string]string {
	out := map[string]string{}
	for _, sub := range arch {
		for _, c := range sub.Components {
			if _, ok := out[program.ComponentKey(c.ID)]; !ok {
				out[program.ComponentKey(c.ID)] = componentPath(sub, c)
			}
		}
	}
	return out
}

// lifecycle is a component's state as a reader reads it, the architecture
// status page's states by one rule (srd045 R8.5): actual with the tag that
// shipped it, in transition with the release its work lands in, else
// intended, and deferred when its status says so.
func lifecycle(c timeline.ComponentFact) string {
	switch c.State {
	case backlog.Actual:
		if c.Tag != "" {
			return "actual as of " + c.Tag
		}
		return "actual"
	case backlog.InTransition:
		if c.Release != "" {
			return "in transition, its work lands in release " + c.Release
		}
		return "in transition"
	case backlog.Intended:
		if c.Deferred {
			return "intended, deferred"
		}
		return "intended"
	}
	return "not declared by the system's ARCHITECTURE"
}

// subsystemState is a subsystem's lifecycle state, from its components:
// in transition while any component is, actual once every judged one is,
// else intended.
func subsystemState(sub timeline.SubsystemFact) string {
	judged, actual := 0, 0
	for _, c := range sub.Components {
		switch c.State {
		case backlog.InTransition:
			return "in transition"
		case backlog.Actual:
			judged, actual = judged+1, actual+1
		case backlog.Intended:
			judged++
		}
	}
	if judged > 0 && judged == actual {
		return "actual"
	}
	return "intended"
}

// contractLine is a contract's link: the site's corpus copy for a system
// document, else its source on the forge where a browser opens it, else
// its path without a link, since no generated page links what does not
// exist (R8.5).
func contractLine(ct *timeline.Contract, page string, corpus func(string) bool) string {
	if ct == nil {
		return ""
	}
	label := ct.Path
	if ct.Team != "" {
		label = ct.Team + "'s " + ct.Path
	}
	if rel, ok := strings.CutPrefix(ct.Path, "docs/"); ok && ct.Team == "" && corpus(rel) {
		return fmt.Sprintf("[%s](%s)", label, relLink(page, CorpusDir+"/"+rel))
	}
	if strings.HasPrefix(ct.Source, "https://") || strings.HasPrefix(ct.Source, "http://") {
		return fmt.Sprintf("[%s](%s)", label, ct.Source)
	}
	return label
}

// ownerLine is who owns a subsystem's work: its scrum's page, or the
// initiative's page when its SRD declares an initiative's repository
// (R8.6, GH-2811).
func ownerLine(sub timeline.SubsystemFact, page string) string {
	switch {
	case sub.Initiative != "":
		return fmt.Sprintf("Its repository is [%s](%s)'s, an initiative's: its epics and features are on the plan under it.", sub.Initiative, relLink(page, HierarchyDir+"/"+sub.Initiative+".md"))
	case sub.Team != "":
		return fmt.Sprintf("The scrum owning it is [%s](%s).", sub.Team, relLink(page, ScrumsDir+"/"+sub.Team+".md"))
	}
	return ""
}

// architectureIndex is the folder's index, the tree (R9.1): the system,
// linking the corpus copy of its ARCHITECTURE.md, each subsystem of the
// registry, and its components, each linking its page.
func architectureIndex(f timeline.Facts, corpus func(string) bool) string {
	var b strings.Builder
	b.WriteString(generated("architecture", "the timeline facts"))
	b.WriteString("# Architecture\n\nThe system's subsystems and their components, as the registry's subsystem SRDs declare them: each component with its lifecycle state and its contract, the component SRD its repository holds. The plan says what is being built; these pages say what it is built into, and the citations join them.\n\n")
	b.WriteString("## The system\n\n")
	if corpus("ARCHITECTURE.md") {
		b.WriteString("- [ARCHITECTURE](../corpus/ARCHITECTURE.md): the intended state, whose components these pages place\n\n")
	} else {
		b.WriteString("- ARCHITECTURE: the system repository holds none the site copies\n\n")
	}
	b.WriteString("## Subsystems\n\n")
	if len(f.Architecture) == 0 {
		b.WriteString("No subsystem SRD declares one yet.\n")
		return b.String()
	}
	for _, sub := range f.Architecture {
		line := fmt.Sprintf("- [%s](%s.md)", sub.ID, sub.ID)
		if sub.Owner != "" {
			line += ": " + sub.Owner
		}
		b.WriteString(line + "\n")
		for _, c := range sub.Components {
			fmt.Fprintf(&b, "  - [%s](%s/%s.md): %s\n", c.ID, sub.ID, c.ID, lifecycle(c))
		}
	}
	return b.String()
}

// subsystemPage is one subsystem's page: its owner, its contract, the
// subsystem SRD, its lifecycle state, the scrum owning it, and its
// components (R9.1).
func subsystemPage(sub timeline.SubsystemFact, corpus func(string) bool) string {
	page := subsystemPath(sub)
	var b strings.Builder
	b.WriteString(generated("architecture", "the timeline facts"))
	b.WriteString(`<div class="crumbs">` + "\n\n[Architecture](" + relLink(page, ArchitectureDir+"/index.md") + ")\n\n</div>\n\n")
	fmt.Fprintf(&b, "# %s\n\n", sub.ID)
	facts := []string{"subsystem", subsystemState(sub)}
	if sub.Owner != "" {
		facts = append(facts, sub.Owner)
	}
	b.WriteString(strings.Join(facts, " · ") + "\n\n")
	if line := ownerLine(sub, page); line != "" {
		b.WriteString(line + "\n\n")
	}
	b.WriteString("## Contract\n\n")
	if line := contractLine(sub.Contract, page, corpus); line != "" {
		b.WriteString("- " + line + ": the subsystem SRD, declaring the subsystem, its owner, and its components\n\n")
	} else {
		b.WriteString("Its subsystem SRD is not read; the facts' notes say why.\n\n")
	}
	b.WriteString("## Components\n\n")
	if len(sub.Components) == 0 {
		b.WriteString("Its SRD lists no component yet.\n")
	}
	for _, c := range sub.Components {
		fmt.Fprintf(&b, "- [%s](%s): %s\n", c.ID, relLink(page, componentPath(sub, c)), lifecycle(c))
	}
	return b.String()
}

// componentPage is one component's page: its lifecycle state with its tag
// or release, the same states the architecture status page shows, its
// subsystem, the scrum owning it, its contract, and the planning documents
// whose architecture citations name it (R9.1).
func componentPage(sub timeline.SubsystemFact, c timeline.ComponentFact, f timeline.Facts) string {
	page := componentPath(sub, c)
	var b strings.Builder
	b.WriteString(generated("architecture", "the timeline facts"))
	fmt.Fprintf(&b, `<div class="crumbs">`+"\n\n[Architecture](%s) › [%s](%s)\n\n</div>\n\n",
		relLink(page, ArchitectureDir+"/index.md"), sub.ID, relLink(page, subsystemPath(sub)))
	fmt.Fprintf(&b, "# %s\n\n", c.ID)
	facts := []string{"component", lifecycle(c)}
	if c.Repository != "" {
		facts = append(facts, "in "+c.Repository)
	}
	b.WriteString(strings.Join(facts, " · ") + "\n\n")
	fmt.Fprintf(&b, "## Up\n\n- [%s](%s), its subsystem\n\n", sub.ID, relLink(page, subsystemPath(sub)))
	if line := ownerLine(sub, page); line != "" {
		b.WriteString(line + "\n\n")
	}
	b.WriteString("## Contract\n\n")
	corpusNone := func(string) bool { return false }
	if line := contractLine(c.Contract, page, corpusNone); line != "" {
		b.WriteString("- " + line + ": the component SRD, whose items its stories cite\n\n")
	} else {
		b.WriteString("Its repository holds no component SRD yet; its software requirements still sit in a subsystem SRD, which a story cites where they live.\n\n")
	}
	b.WriteString("## Cited by\n\n")
	cited := false
	for _, it := range f.Items {
		if !citesComponent(it, c.ID) {
			continue
		}
		cited = true
		line := fmt.Sprintf("- %s [%s](%s): %s", badge(it.Kind), it.Title, relLink(page, HierarchyDir+"/"+it.ID+".md"), it.Status)
		if it.Points > 0 {
			line += fmt.Sprintf(", %d of %d points", it.PointsDone, it.Points)
		}
		b.WriteString(line + "\n")
	}
	if !cited {
		b.WriteString("No planning document cites it yet.\n")
	} else {
		b.WriteString("\n")
	}
	return b.String()
}

// citesComponent reports whether a planning document's architecture
// citations name the component, compared as labels are; a citation by URL
// is another repository's component, never this one.
func citesComponent(it timeline.Item, component string) bool {
	if !hierarchyKind(it.Kind) {
		return false
	}
	for _, a := range it.Architecture {
		if !strings.Contains(a, "://") && program.ComponentKey(a) == program.ComponentKey(component) {
			return true
		}
	}
	return false
}

// hierarchyKind reports whether a kind is a planning document's.
func hierarchyKind(kind string) bool {
	for _, k := range hierarchyKinds {
		if k == kind {
			return true
		}
	}
	return false
}
