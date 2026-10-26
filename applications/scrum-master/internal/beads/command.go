// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	cli.Default.Register(&cli.Command{
		Group: "tracker", Name: "reconcile",
		Summary: "push writes queued offline and confirm or lose provisional pops (beads tracker)",
		Run: func(ctx *cli.Context) error {
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			b, ok := t.(*Tracker)
			if !ok {
				return fmt.Errorf("scrum-master tracker reconcile serves the beads tracker; set SCRUM_MASTER_TRACKER")
			}
			if ctx.DryRun {
				return fmt.Errorf("%w: reconcile pushes queued writes and has no dry run", cli.ErrUsage)
			}
			r, err := b.Store.Reconcile()
			if err != nil {
				return err
			}
			lines := []string{fmt.Sprintf("%d queued writes pushed; the tracker is at %s", len(r.Pushed), r.Commit)}
			for _, w := range r.Pushed {
				lines = append(lines, "pushed: "+w)
			}
			lines = append(lines, fmt.Sprintf("%d provisional pops reconciled", len(r.Reconciled)))
			for _, x := range r.Reconciled {
				lines = append(lines, x.ID+": "+x.Outcome)
			}
			return ctx.Emit(r, strings.Join(lines, "\n"))
		},
	})

	var exportOut string
	cli.Default.Register(&cli.Command{
		Group: "tracker", Name: "migrate",
		Summary: "move a tracker from one JSONL file to one file per issue with an IID index, once, when no one is writing",
		Run: func(ctx *cli.Context) error {
			b, err := beadsTracker(ctx)
			if err != nil {
				return err
			}
			if ctx.DryRun {
				return fmt.Errorf("%w: migrate has no dry run; scrum-master tracker export shows the issues it would move", cli.ErrUsage)
			}
			r, err := b.Store.Migrate()
			if err != nil {
				return err
			}
			human := fmt.Sprintf("moved %d issues to one file each; the tracker is at %s", r.Issues, r.Commit)
			switch {
			case r.Already && r.Indexed > 0:
				human = fmt.Sprintf("indexed the tracker's %d issues; the tracker is at %s", r.Indexed, r.Commit)
			case r.Already:
				human = "the tracker already keeps one file per issue, with its index"
			}
			return ctx.Emit(r, human)
		},
	})
	cli.Default.Register(&cli.Command{
		Group: "tracker", Name: "export",
		Summary: "write every issue as bd's JSONL, for bd import",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&exportOut, "out", "", "the file to write; empty writes to standard output")
		},
		Run: func(ctx *cli.Context) error {
			b, err := beadsTracker(ctx)
			if err != nil {
				return err
			}
			data, err := b.Store.Export()
			if err != nil {
				return err
			}
			if exportOut == "" {
				_, err := ctx.Out.Write(data)
				return err
			}
			if err := os.WriteFile(exportOut, data, 0o644); err != nil {
				return err
			}
			return ctx.Emit(struct {
				Path  string `json:"path"`
				Bytes int    `json:"bytes"`
			}{exportOut, len(data)}, fmt.Sprintf("wrote %s", exportOut))
		},
	})
}

// beadsTracker is the context's tracker as the beads tracker the tracker
// commands serve.
func beadsTracker(ctx *cli.Context) (*Tracker, error) {
	t, err := ctx.Tracker()
	if err != nil {
		return nil, err
	}
	if r, routed := t.(*tracker.Router); routed {
		t = r.Default
	}
	b, ok := t.(*Tracker)
	if !ok {
		return nil, fmt.Errorf("the tracker commands serve the beads tracker; set SCRUM_MASTER_TRACKER")
	}
	return b, nil
}
