// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package goals

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// WebBase is the web root repositories are cited under: a pinned citation
// is <web base>/<project>/-/blob/<tag>/<path> (srd007). SCRUM_MASTER_WEB_BASE
// names it; when unset, it is the root of the checkout at dir's origin as
// configured, not as url.insteadOf expands it (GH-93, GH-105), since every
// repository of a program is on the one forge (GH-109).
func WebBase(dir string) (string, error) {
	if base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("SCRUM_MASTER_WEB_BASE")), "/"); base != "" { //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
		return base, nil
	}
	if base := originWebRoot(dir); base != "" {
		return base, nil
	}
	return "", errors.New("no web base for citations: set SCRUM_MASTER_WEB_BASE to the web root repositories are cited under, or run from a checkout whose origin is an https or ssh URL")
}

// originWebRoot is the scheme and host of dir's origin, as a browser opens
// them: https://host for an https or scp-style ssh origin, empty for any
// other.
func originWebRoot(dir string) string {
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	origin := strings.TrimSpace(string(out))
	if rest, ok := strings.CutPrefix(origin, "https://"); ok {
		host, _, _ := strings.Cut(rest, "/")
		if _, h, ok := strings.Cut(host, "@"); ok {
			host = h
		}
		return "https://" + host
	}
	if rest, ok := strings.CutPrefix(origin, "ssh://"); ok {
		host, _, _ := strings.Cut(rest, "/")
		if _, h, ok := strings.Cut(host, "@"); ok {
			host = h
		}
		host, _, _ = strings.Cut(host, ":")
		return "https://" + host
	}
	if user, rest, ok := strings.Cut(origin, "@"); ok && !strings.Contains(user, "/") {
		if host, path, ok := strings.Cut(rest, ":"); ok && path != "" && !strings.Contains(host, "/") {
			return "https://" + host
		}
	}
	return ""
}

func init() {
	var a struct{ system, release, ref, planFile, control, dir string }
	common := func(fs *flag.FlagSet) {
		fs.StringVar(&a.system, "system", "", "system repository path (required)")
		fs.StringVar(&a.release, "release", "", "road-map release version (required)")
		fs.StringVar(&a.ref, "ref", "main", "ref of the road map, docs/road-map.md")
		fs.StringVar(&a.dir, "dir", ".", "the checkout the tracker is routed from, as the other commands a step runs from a checkout take it")
	}
	load := func(ctx *cli.Context) (tracker.Tracker, Sources, error) {
		if a.system == "" || a.release == "" {
			return nil, Sources{}, fmt.Errorf("%w: needs --system and --release", cli.ErrUsage)
		}
		// goals reads the corpus through the tracker, which scrum-master routes from
		// the working directory.
		if a.dir != "." {
			if err := os.Chdir(a.dir); err != nil {
				return nil, Sources{}, err
			}
		}
		t, err := ctx.Tracker()
		if err != nil {
			return nil, Sources{}, err
		}
		s, err := Load(t, a.system, a.release, a.ref)
		return t, s, err
	}
	cli.Default.Register(&cli.Command{
		Group: "goals", Name: "plan", Summary: "initiative outcomes of a release with recommended team and environment",
		Flags: func(fs *flag.FlagSet) {
			common(fs)
			// plan reads no control files; it takes --control so the one
			// flag set serves plan and push, as AGENTS.md asks (GH-20).
			fs.StringVar(&a.control, "control", "", "program repository holding control/ files; plan reads none, and accepts it so one flag set serves plan and push")
		},
		Run: func(ctx *cli.Context) error {
			t, s, err := load(ctx)
			if err != nil {
				return err
			}
			p, err := MakePlan(t, s)
			if err != nil {
				return err
			}
			var h strings.Builder
			for _, o := range p.Objectives {
				line := fmt.Sprintf("%s %q items=%s -> repository=%q environment=%s due=%s existing=%s", o.Key, o.Title, strings.Join(o.Items, ","), o.Repository, o.Environment, o.Due, o.Existing)
				if o.Carried != "" {
					line += " carried by " + o.Carried
				}
				fmt.Fprintln(&h, line)
			}
			fmt.Fprintln(&h, p.Pushes)
			return ctx.Emit(p, h.String())
		},
	})
	cli.Default.Register(&cli.Command{
		Group: "goals", Name: "push", Summary: "file one Sprint Goal per team and post planned events",
		Flags: func(fs *flag.FlagSet) {
			common(fs)
			fs.StringVar(&a.planFile, "plan", "", "confirmed plan JSON from scrum-master goals plan (required)")
			fs.StringVar(&a.control, "control", "", "program repository holding control/ files; defaults to --system")
		},
		Run: func(ctx *cli.Context) error {
			if a.planFile == "" {
				return fmt.Errorf("%w: push needs --plan", cli.ErrUsage)
			}
			raw, err := os.ReadFile(a.planFile)
			if err != nil {
				return err
			}
			var p Plan
			if err := json.Unmarshal(raw, &p); err != nil {
				return fmt.Errorf("%s: %w", a.planFile, err)
			}
			t, s, err := load(ctx)
			if err != nil {
				return err
			}
			if p.Release != s.Release.Version || p.Tag != s.Release.Tag {
				return fmt.Errorf("plan is for %s at %s, the road-map has %s at %s; run scrum-master goals plan again", p.Release, p.Tag, s.Release.Version, s.Release.Tag)
			}
			control := a.control
			if control == "" {
				control = a.system
			}
			policy, err := program.LoadPolicy(t, control, a.ref)
			if err != nil {
				return err
			}
			web, err := WebBase(".")
			if err != nil {
				return err
			}
			res, err := Push(t, s, p, web, policy, ctx.DryRun)
			if err != nil {
				return err
			}
			var h strings.Builder
			for _, r := range res {
				fmt.Fprintln(&h, pushedLine(r))
				for _, w := range r.Warnings {
					fmt.Fprintf(&h, "  warning: %s\n", w)
				}
			}
			return ctx.Emit(res, h.String())
		},
	})
}

// pushedLine is goals push's human line for one outcome: its key, what push
// did, the Sprint Goal as project#iid with its web address when the tracker
// has one, and any reasons.
func pushedLine(r Pushed) string {
	parts := []string{r.Key + ":", r.Action}
	if r.IID != 0 {
		parts = append(parts, fmt.Sprintf("%s#%d", r.Project, r.IID))
	}
	if strings.Contains(r.WebURL, "://") {
		parts = append(parts, r.WebURL)
	}
	if len(r.Reasons) > 0 {
		parts = append(parts, strings.Join(r.Reasons, "; "))
	}
	return strings.Join(parts, " ")
}
