// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package people

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
)

// systemDir is the system repository's checkout at or beside dir: the one
// holding control/trackers.md, as every word finds it.
func systemDir(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	trackers, err := checkout.FindTrackers(dir)
	if err != nil {
		return "", err
	}
	if trackers == "" {
		return "", fmt.Errorf("no system repository's checkout at or beside %s holds %s", dir, checkout.TrackersFile)
	}
	return filepath.Dir(filepath.Dir(trackers)), nil
}

// changeCommand registers allocate or release, which differ in what they do
// to the names they are given.
func changeCommand(name, summary string, release bool) {
	var a struct {
		dir, person   string
		scrums, epics []string
		commit        bool
	}
	cli.Default.Register(&cli.Command{
		Group: "people", Name: name, Summary: summary,
		Flags: func(fs *flag.FlagSet) {
			// A word may run several times in one process; only the flag
			// setters below keep what they were given.
			a.scrums, a.epics = nil, nil
			fs.StringVar(&a.dir, "dir", ".", "the system repository's checkout, or a checkout beside it")
			fs.StringVar(&a.person, "person", "", "the person's tracker username (required)")
			fs.Func("scrum", "a scrum, as the site names it (repeatable)", func(v string) error {
				a.scrums = append(a.scrums, v)
				return nil
			})
			fs.Func("epic", "an epic, as the site names it (repeatable)", func(v string) error {
				a.epics = append(a.epics, v)
				return nil
			})
			fs.BoolVar(&a.commit, "commit", false, "commit the changed document on the current branch")
		},
		Run: func(ctx *cli.Context) error {
			if a.person == "" || len(a.scrums)+len(a.epics) == 0 {
				return fmt.Errorf("%w: %s needs --person and at least one --scrum or --epic", cli.ErrUsage, name)
			}
			if strings.ContainsAny(a.person, `/\`) {
				return fmt.Errorf("%w: %q is not a username", cli.ErrUsage, a.person)
			}
			dir, err := systemDir(a.dir)
			if err != nil {
				return err
			}
			r, err := Run(dir, Change{Username: a.person, Scrums: a.scrums, Epics: a.epics, Release: release}, a.commit, ctx.DryRun)
			if err != nil {
				return err
			}
			action := r.Action
			if r.DryRun && action != "unchanged" {
				action = "would be " + action
			}
			h := fmt.Sprintf("%s %s: %s\n", r.Path, a.person, action)
			if r.Commit != "" {
				h += "committed " + r.Commit + "\n"
			}
			return ctx.Emit(r, h)
		},
	})
}

// reviewCommand registers review or unreview, which name who signs off on a
// scrum or document.
func reviewCommand(name, summary string, release bool) {
	var a struct {
		dir, person       string
		scrums, documents []string
		commit, force     bool
	}
	cli.Default.Register(&cli.Command{
		Group: "people", Name: name, Summary: summary,
		Flags: func(fs *flag.FlagSet) {
			a.scrums, a.documents = nil, nil
			fs.StringVar(&a.dir, "dir", ".", "the system repository's checkout, or a checkout beside it")
			fs.StringVar(&a.person, "person", "", "the person's tracker username (required)")
			fs.Func("scrum", "a scrum, as the site names it (repeatable)", func(v string) error {
				a.scrums = append(a.scrums, v)
				return nil
			})
			fs.Func("document", "a story, feature, epic, or SRD, as the site names it (repeatable)", func(v string) error {
				a.documents = append(a.documents, v)
				return nil
			})
			fs.BoolVar(&a.commit, "commit", false, "commit the changed document on the current branch")
			if !release {
				fs.BoolVar(&a.force, "force", false, "move a name another person reviews")
			}
		},
		Run: func(ctx *cli.Context) error {
			if a.person == "" || len(a.scrums)+len(a.documents) == 0 {
				return fmt.Errorf("%w: %s needs --person and at least one --scrum or --document", cli.ErrUsage, name)
			}
			if strings.ContainsAny(a.person, `/\`) {
				return fmt.Errorf("%w: %q is not a username", cli.ErrUsage, a.person)
			}
			dir, err := systemDir(a.dir)
			if err != nil {
				return err
			}
			names := append(append([]string{}, a.scrums...), a.documents...)
			r, err := Run(dir, Change{Username: a.person, Reviews: names, Release: release, Force: a.force}, a.commit, ctx.DryRun)
			if err != nil {
				return err
			}
			action := r.Action
			if r.DryRun && action != "unchanged" {
				action = "would be " + action
			}
			h := fmt.Sprintf("%s %s: %s\n", r.Path, a.person, action)
			if r.Commit != "" {
				h += "committed " + r.Commit + "\n"
			}
			return ctx.Emit(r, h)
		},
	})
}

func init() {
	changeCommand("allocate", "name a person on a scrum or epic in control/people.md; it assigns no issue", false)
	reviewCommand("review", "name who signs off on a scrum or document in control/people.md; agents never run it", false)
	reviewCommand("unreview", "take a person off signing off on a scrum or document in control/people.md; agents never run it", true)
	changeCommand("release", "take a person off a scrum or epic in control/people.md", true)

	var la struct{ dir string }
	cli.Default.Register(&cli.Command{
		Group: "people", Name: "list", Summary: "who control/people.md names on which scrums and epics, and who signs off on what",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&la.dir, "dir", ".", "the system repository's checkout, or a checkout beside it")
		},
		Run: func(ctx *cli.Context) error {
			dir, err := systemDir(la.dir)
			if err != nil {
				return err
			}
			c, err := Load(dir)
			if err != nil {
				return err
			}
			var h strings.Builder
			if len(c.People) == 0 {
				fmt.Fprintf(&h, "%s names no one\n", ControlFile)
			}
			for _, p := range c.People {
				fmt.Fprintf(&h, "%s: scrums %s; epics %s; reviews %s\n", p.Username, orNone(p.Scrums), orNone(p.Epics), orNone(p.Reviews))
			}
			return ctx.Emit(c, h.String())
		},
	})
}

func orNone(n Names) string {
	if len(n) == 0 {
		return "none"
	}
	return strings.Join(n, ", ")
}
