// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// CommitsSince returns the messages of the commits in dir since tag, or of
// every commit when there is no tag yet.
func CommitsSince(dir, tag string) ([]string, error) {
	args := []string{"-C", dir, "log", "--format=%B%x1e"}
	if tag != "" {
		args = append(args, tag+"..HEAD")
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("read the commits since %s: %w", tag, err)
	}
	var messages []string
	for _, m := range strings.Split(string(out), "\x1e") {
		if m = strings.TrimSpace(m); m != "" {
			messages = append(messages, m)
		}
	}
	return messages, nil
}

// SRDDir holds a corpus's requirements.
const SRDDir = "docs/specs/software-requirements"

// RequirementPaths are where a corpus holds its requirements: the SRDs, the
// initiatives and objectives a system plans from, and the road map a plan
// writes (srd015 R3.1).
var RequirementPaths = []string{SRDDir, "docs/specs/initiatives", "docs/specs/objectives", "docs/road-map.md"}

// RequirementsUnchanged reports whether the checkout at dir holds a corpus
// whose requirements no commit since tag changed: a republish of the same
// requirements. A checkout with no SRDs, such as a code repository, or no
// tag yet, is never one.
func RequirementsUnchanged(dir, tag string) (bool, error) {
	if tag == "" {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(dir, SRDDir)); err != nil {
		return false, nil
	}
	cmd := exec.Command("git", append([]string{"-C", dir, "diff", "--quiet", tag + "..HEAD", "--"}, RequirementPaths...)...)
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	}
	return false, fmt.Errorf("compare the requirements with %s: %w", tag, err)
}

// Recommendation is Next for the checkout at dir with its recommendation
// from the commits since the latest tag (srd015 R3.1).
func Recommendation(r program.Releases, artifact, dir string, tags []string) (NextResult, error) {
	n := Next(r, artifact, tags)
	messages, err := CommitsSince(dir, n.LatestTag)
	if err != nil {
		return n, err
	}
	unchanged, err := RequirementsUnchanged(dir, n.LatestTag)
	if err != nil {
		return n, err
	}
	n.Recommend(messages, unchanged)
	return n, nil
}

// RepoTags are the tags in the checkout at dir and on its origin.
func RepoTags(dir string) ([]string, error) {
	git := ExecGit(dir)
	local, err := git("tag", "--list")
	if err != nil {
		return nil, err
	}
	remote, err := RemoteTags(git, "*")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.Fields(local + "\n" + remote) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out, nil
}

func init() {
	var args struct {
		artifact, control, ref, dir, chosen string
		requireVersion                      bool
	}
	common := func(fs *flag.FlagSet) {
		fs.StringVar(&args.artifact, "artifact", "", "artifact a subsystem SRD declares (required)")
		fs.StringVar(&args.control, "control", "", "program repository holding control/policy.md; empty is the system repository beside the checkout, else the defaults")
		fs.StringVar(&args.ref, "ref", "main", "ref of the control files")
		fs.StringVar(&args.dir, "dir", ".", "the artifact repository's local checkout")
	}
	policy := func(ctx *cli.Context) (program.Policy, error) {
		if args.artifact == "" {
			return program.Policy{}, fmt.Errorf("%w: name the artifact with --artifact", cli.ErrUsage)
		}
		control := program.ControlFor(args.control, args.dir, "")
		if control == "" {
			return program.Defaults(), nil
		}
		t, err := ctx.Tracker()
		if err != nil {
			return program.Policy{}, err
		}
		return program.LoadPolicy(t, control, args.ref)
	}

	var gateArgs struct{ version, issue, project string }
	cli.Default.Register(&cli.Command{
		Group: "release", Name: "gate", Summary: "run the artifact's declared checks and post their evidence on the release issue",
		Flags: func(fs *flag.FlagSet) {
			common(fs)
			fs.StringVar(&gateArgs.version, "version", "", "candidate version major.minor.patch (required)")
			fs.StringVar(&gateArgs.issue, "issue", "", "release issue as project#iid, or its iid with --project (required)")
			fs.StringVar(&gateArgs.project, "project", "", "the project a bare --issue iid is in")
		},
		Run: func(ctx *cli.Context) error {
			p, err := policy(ctx)
			if err != nil {
				return err
			}
			v, ok := ParseVersion(gateArgs.version)
			if !ok {
				return fmt.Errorf("%w: --version must be major.minor.patch, not %q", cli.ErrUsage, gateArgs.version)
			}
			project, iid, err := IssueRef(gateArgs.issue, gateArgs.project)
			if err != nil {
				return fmt.Errorf("%w: %v", cli.ErrUsage, err)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			out := ctx.Out
			if ctx.JSON {
				out = io.Discard
			}
			r, err := RunGate(t, ReleaseGateInput{Project: project, Issue: iid, Artifact: args.artifact, Version: v, Dir: args.dir,
				Policy: p, Git: ExecGit(args.dir), Exec: ShellExec, Out: out, Now: time.Now(), DryRun: ctx.DryRun})
			if err != nil {
				return err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "gate for %s at %s:\n", r.Tag, short(r.Revision))
			if r.IssueClosed {
				fmt.Fprintf(&b, "note: the release issue %s#%d is closed; its evidence is recorded there, as after the merge that delivered it\n", r.Project, r.Issue)
			}
			for _, c := range r.Checks {
				b.WriteString(gateLine(c) + "\n")
			}
			if r.AwaitingPeerRead {
				fmt.Fprintf(&b, "every check but the peer read passed; %s is tagged after a reviewer's peer read, the peer-read skill\n", r.Tag)
			}
			if err := ctx.Emit(r, b.String()); err != nil {
				return err
			}
			if !r.Passed && !r.DryRun && !r.AwaitingPeerRead {
				return fmt.Errorf("the gate for %s did not pass; no tag follows it", r.Tag)
			}
			return nil
		},
	})

	var tagArgs struct{ version, issue, project string }
	cli.Default.Register(&cli.Command{
		Group: "release", Name: "tag", Summary: "tag and push a candidate whose latest gate passed at HEAD",
		Flags: func(fs *flag.FlagSet) {
			common(fs)
			fs.StringVar(&tagArgs.version, "version", "", "candidate version major.minor.patch (required)")
			fs.StringVar(&tagArgs.issue, "issue", "", "release issue as project#iid, or its iid with --project (required)")
			fs.StringVar(&tagArgs.project, "project", "", "the project a bare --issue iid is in")
		},
		Run: func(ctx *cli.Context) error {
			p, err := policy(ctx)
			if err != nil {
				return err
			}
			v, ok := ParseVersion(tagArgs.version)
			if !ok {
				return fmt.Errorf("%w: --version must be major.minor.patch, not %q", cli.ErrUsage, tagArgs.version)
			}
			project, iid, err := IssueRef(tagArgs.issue, tagArgs.project)
			if err != nil {
				return fmt.Errorf("%w: %v", cli.ErrUsage, err)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := ReleaseTag(t, TagInput{Project: project, Issue: iid, Artifact: args.artifact, Version: v,
				Policy: p, Git: ExecGit(args.dir), DryRun: ctx.DryRun})
			if err != nil {
				return err
			}
			human := fmt.Sprintf("tagged and pushed %s at %s: %s", r.Tag, short(r.Revision), r.Label)
			if r.DryRun {
				human = fmt.Sprintf("would tag %s at %s: %s", r.Tag, short(r.Revision), r.Label)
			}
			return ctx.Emit(r, human)
		},
	})

	cli.Default.Register(&cli.Command{
		Group: "release", Name: "next", Summary: "report the latest version and the next candidates with their class",
		Flags: func(fs *flag.FlagSet) {
			common(fs)
			fs.StringVar(&args.chosen, "version", "", "a candidate version to release in place of the recommendation (GH-47)")
			fs.BoolVar(&args.requireVersion, "require-version", false, "refuse, with the reason, when nothing is recommended or chosen, as the release profile needs a version to gate (GH-64)")
		},
		Run: func(ctx *cli.Context) error {
			p, err := policy(ctx)
			if err != nil {
				return err
			}
			tags, err := RepoTags(args.dir)
			if err != nil {
				return err
			}
			n, err := Recommendation(p.Releases, args.artifact, args.dir, tags)
			if err != nil {
				return err
			}
			if args.chosen != "" {
				if err := n.Choose(args.chosen); err != nil {
					return err
				}
			}
			if err := ctx.Emit(n, n.Human()); err != nil {
				return err
			}
			if args.requireVersion && n.RecommendedVersion == "" {
				return fmt.Errorf("nothing to release for %s: %s", n.Artifact, n.Reason)
			}
			return nil
		},
	})
}

// gateLine is one check of the gate as scrum-master release gate prints it: the
// result, the test, the environment when one applies, and the reason when
// there is one, with no trailing space (srd015 R1.3). A peer read has no
// environment; its "none" stays in the JSON and the evidence event.
func gateLine(c CheckResult) string {
	line := fmt.Sprintf("  %-8s %s", c.Result, c.Test)
	if c.Environment != "" && c.Environment != "none" {
		line += " in " + c.Environment
	}
	if c.Reason != "" {
		line += " " + c.Reason
	}
	return line
}
