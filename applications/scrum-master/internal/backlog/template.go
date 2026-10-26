// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog

import (
	"embed"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// templates are one document of each hierarchy level, in the layout the
// gate reads, chained theme to story so together they pass it (GH-20), and
// VISION and ARCHITECTURE, whose shape the gate also checks (GH-22).
//
//go:embed templates/*.md.tmpl
var templates embed.FS

// Template is a document of kind in the layout scrum-master backlog check accepts:
// its frontmatter, its context tree, and every field the level declares,
// with example values to replace.
func Template(kind string) (string, error) {
	b, err := templates.ReadFile("templates/" + kind + ".md.tmpl")
	if err != nil {
		return "", fmt.Errorf("no template for kind %q; --kind is one of %s", kind, strings.Join(templateKinds(), ", "))
	}
	return string(b), nil
}

// TemplateFor is the template of kind for the repository checked out at
// dir: an epic's owner is the technical program manager in the system
// repository, where cross-team epics live, and the team's Product Owner in
// a team's repository, named by its subsystem SRD (GH-2726).
func TemplateFor(kind, dir string) (string, error) {
	t, err := Template(kind)
	if err != nil || kind != string(hierarchy.Epic) {
		return t, err
	}
	const fixture = "- owner: Product Owner, po-1"
	trackers, err := checkout.FindTrackers(dir)
	if err != nil || trackers == "" {
		return t, nil
	}
	systemDir := filepath.Dir(filepath.Dir(trackers))
	if sameDir(dir, systemDir) {
		return strings.Replace(t, fixture, "- owner: technical program manager", 1), nil
	}
	subs, err := program.SubsystemsIn(systemDir)
	if err != nil {
		return t, nil
	}
	if sub, ok := repositorySubsystem(dir, subs.Teams(program.SystemControl(dir))); ok {
		return strings.Replace(t, fixture, "- owner: Product Owner, "+sub.Owner, 1), nil
	}
	return t, nil
}

// templateKinds are the hierarchy levels, then the documents beside them.
func templateKinds() []string { return append(kinds(), "vision", "architecture") }

func init() {
	var kind, dir string
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "template", Summary: "a document of one hierarchy level, or VISION or ARCHITECTURE, in the layout the gate reads, to copy and fill",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&kind, "kind", "", "the document: "+strings.Join(templateKinds(), ", ")+" (required)")
			fs.StringVar(&dir, "dir", ".", "the repository the document goes in, which names an epic's owner")
		},
		Run: func(ctx *cli.Context) error {
			if kind == "" {
				return fmt.Errorf("%w: template needs --kind, one of %s", cli.ErrUsage, strings.Join(templateKinds(), ", "))
			}
			t, err := TemplateFor(kind, dir)
			if err != nil {
				return err
			}
			return ctx.Emit(struct {
				Kind     string `json:"kind"`
				Document string `json:"document"`
			}{kind, t}, t)
		},
	})
}
