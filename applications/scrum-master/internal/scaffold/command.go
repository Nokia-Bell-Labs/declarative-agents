// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/legacyenv"
)

func init() {
	var (
		o               InitOptions
		agents          []string
		requireProfiles bool
	)
	cli.Default.Register(&cli.Command{
		Group:   "init",
		Summary: "lay down a program's system repository: corpus, control files, skills",
		Flags: func(fs *flag.FlagSet) {
			o, agents = InitOptions{}, nil
			fs.StringVar(&o.Dir, "dir", "", "the system repository's checkout (default: the git root)")
			fs.Func("agent", "install the skills for cursor, claude, copilot, all, or detect, the agents the repository already uses (repeatable)", func(v string) error {
				agents = append(agents, v)
				return nil
			})
			fs.StringVar(&o.Tracker, "tracker", "", "a tracker repository to seed when it has no commits")
			fs.StringVar(&o.TrackerBranch, "tracker-branch", "", "the branch of --tracker holding the tracker, seeded as an orphan when missing; a dedicated branch of the program's own repository keeps code and tracker in one repository (default: main)")
			fs.StringVar(&o.Group, "group", "", "the program group whose labels --tracker gets")
			fs.StringVar(&o.Prefix, "prefix", "", "the issue ID prefix --tracker gets (default: the last segment of --group)")
			fs.BoolVar(&o.Force, "force", false, "replace files a person edited")
			fs.BoolVar(&requireProfiles, "require-profiles", false, "refuse, writing nothing, when no release's profiles are installed to vendor (the init profile's vendor step needs them)")
		},
		Run: func(ctx *cli.Context) error {
			var err error
			if o.Dir == "" {
				if o.Dir, err = gitRoot(); err != nil {
					return err
				}
			}
			if len(agents) > 0 {
				if o.Agents, err = install.ResolveAgents(agents, o.Dir); err != nil {
					return fmt.Errorf("%w: %v", cli.ErrUsage, err)
				}
			}
			if requireProfiles {
				if err := install.CheckSource(o.Dir); err != nil {
					return err
				}
			}
			o.Version, o.DryRun = install.Version(), ctx.DryRun
			if o.Tracker != "" {
				if o.User, err = person(); err != nil {
					return err
				}
			}
			r, err := Init(o)
			if err != nil {
				return err
			}
			return ctx.Emit(r, humanInit(r))
		},
	})
}

func init() {
	var (
		o               OnboardOptions
		agents          []string
		requireProfiles bool
	)
	cli.Default.Register(&cli.Command{
		Group:   "onboard",
		Summary: "add a repository to a program: its corpus here, its registration in the system repository",
		Flags: func(fs *flag.FlagSet) {
			o, agents = OnboardOptions{}, nil
			fs.StringVar(&o.Dir, "dir", "", "the joining repository's checkout (default: the git root)")
			fs.Func("agent", "install the skills for cursor, claude, copilot, all, or detect, the agents the repository already uses (repeatable)", func(v string) error {
				agents = append(agents, v)
				return nil
			})
			fs.StringVar(&o.System, "system", "", "the system repository the registration is filed in (required)")
			fs.StringVar(&o.Ref, "ref", "main", "ref of the system repository's SRDs and control files")
			fs.StringVar(&o.Subsystem, "subsystem", "", "the subsystem SRD that lists the repository, e.g. srd010-ingest (required)")
			fs.StringVar(&o.Repository, "repository", "", "the repository's URL (default: its origin)")
			fs.StringVar(&o.Path, "path", "/", "the component's path in the repository")
			fs.StringVar(&o.Role, "role", RoleComponent, "what the repository is to its subsystem: component, a component's repository; scrum, the team's scrum repository where it plans; or initiative, an initiative's repository holding its epics and features")
			fs.StringVar(&o.Initiative, "initiative", "", "the initiative document the repository holds the epics and features of, such as in001-reliable-agents (required with --role initiative)")
			fs.StringVar(&o.Owner, "owner", "", "the subsystem's owner, for an SRD the registration asks to create")
			fs.Func("component", "a component of the subsystem, for an SRD the registration asks to create (repeatable, or comma-separated; default: the subsystem's short name)", func(v string) error {
				for _, c := range strings.Split(v, ",") {
					if c = strings.TrimSpace(c); c != "" {
						o.Components = append(o.Components, c)
					}
				}
				return nil
			})
			fs.BoolVar(&o.Force, "force", false, "replace files a person edited")
			fs.BoolVar(&requireProfiles, "require-profiles", false, "refuse, writing nothing, when no release's profiles are installed to vendor (the onboard profile's vendor step needs them)")
		},
		Run: func(ctx *cli.Context) error {
			var err error
			if o.System == "" || o.Subsystem == "" {
				return fmt.Errorf("%w: onboard needs --system and --subsystem", cli.ErrUsage)
			}
			if o.Dir == "" {
				if o.Dir, err = gitRoot(); err != nil {
					return err
				}
			}
			if len(agents) > 0 {
				if o.Agents, err = install.ResolveAgents(agents, o.Dir); err != nil {
					return fmt.Errorf("%w: %v", cli.ErrUsage, err)
				}
			}
			if requireProfiles {
				if err := install.CheckSource(o.Dir); err != nil {
					return err
				}
			}
			if o.Repository == "" {
				out, err := originURL(o.Dir)
				if o.Repository = strings.TrimSpace(string(out)); err != nil || o.Repository == "" {
					return fmt.Errorf("%s has no origin; name the repository's URL with --repository", o.Dir)
				}
			}
			o.Version, o.DryRun = install.Version(), ctx.DryRun
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := Onboard(t, o)
			if err != nil {
				return err
			}
			human := humanFiles(r.Files)
			if len(r.Skills) > 0 {
				// As init does: the agents' skills are installed (GH-20).
				human += fmt.Sprintf("skills: %d files for the named agents\n", skillFiles(r.Skills))
			}
			switch reg := r.Registration; {
			case reg.Issue != nil && reg.Filed:
				human += fmt.Sprintf("registration filed: %s#%d\n", reg.Issue.Project, reg.Issue.IID)
			case reg.Issue != nil:
				human += "dry run: would file the registration\n"
			default:
				human += "no registration filed: " + reg.Reason + "\n"
			}
			return ctx.Emit(r, human)
		},
	})
}

func init() {
	var (
		o           DoctorOptions
		failOnDrift bool
	)
	cli.Default.Register(&cli.Command{
		Group:   "doctor",
		Summary: "bring a program repository up to date after an upgrade of scrum-master: missing files, scrum-master's text, document format, installed skills",
		Flags: func(fs *flag.FlagSet) {
			o = DoctorOptions{}
			fs.StringVar(&o.Dir, "dir", "", "the program repository's checkout (default: the git root)")
			fs.BoolVar(&failOnDrift, "fail-on-drift", false, "exit non-zero when doctor would change anything, as the check profile needs (GH-64)")
		},
		Run: func(ctx *cli.Context) error {
			var err error
			if o.Dir == "" {
				if o.Dir, err = gitRoot(); err != nil {
					return err
				}
			}
			o.Version, o.DryRun = install.Version(), ctx.DryRun
			r, err := Doctor(o)
			if err != nil {
				return err
			}
			if err := ctx.Emit(r, humanDoctor(r)); err != nil {
				return err
			}
			if r.Bootstrap.Refused() {
				return fmt.Errorf("bootstrapping order: %s", r.Bootstrap.Reason)
			}
			if drift := r.Drift(); failOnDrift && len(drift) > 0 {
				return fmt.Errorf("drift: %s; run the doctor skill to repair it", strings.Join(drift, ", "))
			}
			return nil
		},
	})
}

// humanDoctor names what doctor changed, or would change on a dry run, and
// the skills scrum-master install would update; a file left as it was is counted,
// not listed.
func humanDoctor(r DoctorResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s repository %s\n", r.Level, r.Dir)
	kept := 0
	for _, f := range r.Files {
		if f.Action == "kept" && !strings.HasPrefix(f.Reason, "does not parse") {
			kept++
			continue
		}
		fmt.Fprintf(&b, "%-12s %s", f.Action, f.Path)
		if f.Reason != "" {
			fmt.Fprintf(&b, " (%s)", f.Reason)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%d files current\n", kept)
	verb := "repaired"
	if r.DryRun {
		verb = "would repair"
	}
	for _, p := range r.Repair.Repaired {
		fmt.Fprintf(&b, "%-12s %s\n", verb, p)
	}
	for _, f := range r.Repair.Findings {
		fmt.Fprintf(&b, "finding      %s\n", f)
	}
	if len(r.Repair.Untracked) > 0 {
		fmt.Fprintf(&b, "not checked, git does not track them (git add them first): %s\n", strings.Join(r.Repair.Untracked, ", "))
	}
	if tr := r.Trackers; tr != nil {
		fmt.Fprintf(&b, "tracking systems (%s): default %s", tr.File, tr.Default.Name)
		for _, p := range slices.Sorted(maps.Keys(tr.Overrides)) {
			fmt.Fprintf(&b, ", %s %s", p, tr.Overrides[p])
		}
		b.WriteString("\n")
		for _, problem := range tr.Problems {
			fmt.Fprintf(&b, "  %s\n", problem)
		}
	}
	if r.Repair.Skipped != "" {
		fmt.Fprintf(&b, "repair skipped: %s\n", r.Repair.Skipped)
	}
	if r.Vendored == "" {
		fmt.Fprintf(&b, "profiles: none vendored; run the upgrade profile with the agent\n")
	} else {
		fmt.Fprintf(&b, "profiles: %s vendored in %s\n", r.Vendored, install.VendorDir)
	}
	if r.PinDrift != "" {
		fmt.Fprintf(&b, "pin: %s\n", r.PinDrift)
	}
	for _, p := range r.Credentials {
		var vars []string
		for _, v := range p.Vars {
			state := "set"
			if !v.Set {
				state = "unset"
			}
			vars = append(vars, v.Name+" ("+state+")")
		}
		fmt.Fprintf(&b, "profile %s reads: %s\n", p.Profile, strings.Join(vars, ", "))
	}
	for _, name := range r.LegacyEnv {
		fmt.Fprintf(&b, "%s is set: rename it %s; the TPM_ names are read only where the new one is unset\n", name, legacyenv.Renamed(name))
	}
	for _, c := range r.OrphanedClones {
		fmt.Fprintf(&b, "orphaned tracker clone %s: copy what you need from it, then remove it\n", c)
	}
	agents := map[string]bool{}
	for _, f := range r.Skills {
		fmt.Fprintf(&b, "skill %-9s %s %s\n", f.Problem, f.Agent, f.Path)
		agents[f.Agent] = true
	}
	for _, a := range install.Agents {
		if agents[a] {
			fmt.Fprintf(&b, "run scrum-master install --agent %s to update its skills\n", a)
		}
	}
	if bs := r.Bootstrap; bs != nil {
		if bs.Named {
			fmt.Fprintf(&b, "bootstrapping order: the system repository's ARCHITECTURE names %s\n", bs.Subsystem)
		} else {
			fmt.Fprintf(&b, "bootstrapping order: %s\n", bs.Reason)
		}
	}
	if r.DryRun {
		b.WriteString("dry run: nothing written\n")
	}
	return b.String()
}

func humanFiles(files []Result) string {
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "%-12s %s", f.Action, f.Path)
		if f.Reason != "" && f.Reason != "unchanged" {
			fmt.Fprintf(&b, " (%s)", f.Reason)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func humanInit(r InitResult) string {
	var b strings.Builder
	b.WriteString(humanFiles(r.Files))
	if len(r.Skills) > 0 {
		fmt.Fprintf(&b, "skills: %d files for the named agents\n", skillFiles(r.Skills))
	}
	if t := r.Tracker; t != nil {
		switch {
		case t.Seeded:
			fmt.Fprintf(&b, "tracker %s: first commit made, issue prefix %s, %d labels\n", t.Remote, t.Prefix, len(t.Labels))
		case t.Remote == "":
			fmt.Fprintf(&b, "tracker: %s\n", t.Reason)
		default:
			fmt.Fprintf(&b, "tracker %s: %s\n", t.Remote, t.Reason)
		}
	}
	switch {
	case r.Group != "":
		fmt.Fprintf(&b, "program group: %s, recorded in control/trackers.md\n", r.Group)
	case r.GroupReason != "":
		fmt.Fprintf(&b, "program group: not recorded: %s\n", r.GroupReason)
	}
	if r.DryRun {
		b.WriteString("dry run: nothing written\n")
	}
	return b.String()
}

func gitRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository; name one with --dir")
	}
	return strings.TrimSpace(string(out)), nil
}

// person is who the tracker's writes are made as, as the beads tracker
// resolves it: SCRUM_MASTER_USER, else git's user.name.
func person() (string, error) {
	if u := strings.TrimSpace(os.Getenv("SCRUM_MASTER_USER")); u != "" { //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
		return u, nil
	}
	out, err := exec.Command("git", "config", "user.name").Output()
	if u := strings.TrimSpace(string(out)); err == nil && u != "" {
		return u, nil
	}
	return "", fmt.Errorf("no person for the tracker's writes: set SCRUM_MASTER_USER or git config user.name")
}

// skillFiles counts the skill files among an install's results; the
// .git/info/exclude update it also reports is not one (GH-22).
func skillFiles(results []install.Result) int {
	n := 0
	for _, r := range results {
		if r.Skill != "" {
			n++
		}
	}
	return n
}
