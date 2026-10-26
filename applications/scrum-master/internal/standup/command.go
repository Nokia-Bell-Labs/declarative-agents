// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package standup

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

func init() {
	var args struct {
		project, control, ref, due, title, summary, summaryFile string
		iid                                                     int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "recur", Summary: "open the next instance of a recurring issue",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&args.project, "project", "", "project path (required)")
			fs.IntVar(&args.iid, "issue", 0, "the recurring issue's IID (required)")
			fs.StringVar(&args.due, "due", "", "the next due date YYYY-MM-DD; empty adds seven days")
			fs.StringVar(&args.title, "title", "", "the next title; empty keeps the issue's")
			fs.StringVar(&args.summary, "summary", "", "the run's result: closes an open instance and goes on the Previous Runs line")
			fs.StringVar(&args.summaryFile, "summary-file", "", "a file holding the run's result, in place of --summary")
			fs.StringVar(&args.control, "control", "", "project holding control/policy.md, which admits the next instance; empty is the system repository beside the checkout, else --project")
			fs.StringVar(&args.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if args.project == "" || args.iid == 0 {
				return fmt.Errorf("%w: recur needs --project and --issue", cli.ErrUsage)
			}
			summary := args.summary
			if args.summaryFile != "" {
				if summary != "" {
					return fmt.Errorf("%w: give --summary or --summary-file, not both", cli.ErrUsage)
				}
				b, err := os.ReadFile(args.summaryFile)
				if err != nil {
					return err
				}
				summary = string(b)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			p, err := program.LoadPolicy(t, program.ControlFor(args.control, ".", args.project), args.ref)
			if err != nil {
				return err
			}
			r, err := Recur(t, args.project, args.iid, RecurOptions{Title: args.title, DueDate: args.due, Summary: summary, Policy: p, Now: time.Now(), DryRun: ctx.DryRun})
			if err != nil {
				return err
			}
			human := fmt.Sprintf("%s#%d recurs as #%d, due %s", r.Project, r.Previous, r.Next, r.DueDate)
			if r.Existing {
				human = fmt.Sprintf("%s#%d already recurs as #%d", r.Project, r.Previous, r.Next)
			}
			return ctx.Emit(r, human)
		},
	})
}
