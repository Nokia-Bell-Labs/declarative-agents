// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package install

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
)

// Version is the scrum-master version stamped into installed files: the release the
// binary was published at, else its module version, or dev for a local
// build (internal/version).
func Version() string { return version.Current() }

func gitRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository; name one with --dir")
	}
	return strings.TrimSpace(string(out)), nil
}

func rel(dir, path string) string {
	if r, err := filepath.Rel(dir, path); err == nil {
		return filepath.ToSlash(r)
	}
	return path
}

func init() {
	var (
		agents             []string
		dir                string
		list, check, force bool
		profiles           bool
	)
	cli.Default.Register(&cli.Command{
		Group:   "install",
		Summary: "write the embedded skill commands for an agent, or vendor the profiles they run",
		Flags: func(fs *flag.FlagSet) {
			agents = nil
			fs.Func("agent", "cursor, claude, copilot, all, or detect, the agents the repository already uses (repeatable)", func(v string) error {
				agents = append(agents, v)
				return nil
			})
			fs.StringVar(&dir, "dir", "", "repository to install into (default: the git root)")
			fs.BoolVar(&list, "list", false, "name the embedded skills and write nothing")
			fs.BoolVar(&check, "check", false, "report installed files that differ from the binary; write nothing")
			fs.BoolVar(&force, "force", false, "replace files a person edited")
			fs.BoolVar(&profiles, "profiles", false, "vendor the release's profiles and agent-core declarations into "+VendorDir+", which the skills run (GH-29)")
		},
		Run: func(ctx *cli.Context) error {
			if profiles && check {
				return checkVendored(ctx, dir)
			}
			if profiles {
				return vendorProfiles(ctx, dir, force)
			}
			skills, err := EmbeddedSkills()
			if err != nil {
				return err
			}
			if list {
				var b strings.Builder
				for _, s := range skills {
					fmt.Fprintf(&b, "%-20s %s\n", s.Name, s.Description)
				}
				return ctx.Emit(skills, b.String())
			}
			if dir == "" {
				if dir, err = gitRoot(); err != nil {
					return err
				}
			}
			targets, err := ResolveAgents(agents, dir)
			if err != nil {
				return fmt.Errorf("%w: %v", cli.ErrUsage, err)
			}
			version := Version()
			if check {
				findings, err := Check(dir, targets, skills, version)
				if err != nil {
					return err
				}
				var b strings.Builder
				for _, f := range findings {
					fmt.Fprintf(&b, "%-9s %s\n", f.Problem, rel(dir, f.Path))
				}
				if len(findings) == 0 {
					b.WriteString("installed skills match scrum-master " + version)
				}
				if err := ctx.Emit(findings, b.String()); err != nil {
					return err
				}
				if len(findings) > 0 {
					return fmt.Errorf("%d installed skill file(s) differ from scrum-master %s; run scrum-master install", len(findings), version)
				}
				return nil
			}
			results, err := Install(dir, targets, skills, version, force, ctx.DryRun)
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, r := range results {
				fmt.Fprintf(&b, "%-9s %s", r.Action, rel(dir, r.Path))
				if r.Reason != "" {
					fmt.Fprintf(&b, " (%s)", r.Reason)
				}
				b.WriteString("\n")
			}
			return ctx.Emit(results, b.String())
		},
	})
}

// SourceEnv names a declarative-agents source tree to vendor from: a
// development override, as the rig's runs use.
const SourceEnv = "SCRUM_MASTER_SOURCE"

// HomeEnv names where install.sh unpacked a release's profiles: agents/,
// agent-core/tools/, and release (GH-34). By default it is
// $XDG_DATA_HOME/scrum-master, else $HOME/.local/share/scrum-master.
const HomeEnv = "SCRUM_MASTER_HOME"

// Home is where the installed profiles live.
func Home() string {
	if h := os.Getenv(HomeEnv); h != "" { //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
		return h
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" { //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
		return filepath.Join(d, "scrum-master")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "scrum-master")
}

// vendorSource is the release the repository at dir vendors from:
// SourceEnv's tree when set, this repository's own inside the repository
// that ships agent-core, else the release install.sh installed.
func vendorSource(dir string) (VendorSource, error) {
	tree := func(release, root string) VendorSource {
		agents := filepath.Join(root, "applications", "scrum-master", "plugin", "agents")
		if _, err := os.Stat(agents); err != nil {
			agents = filepath.Join(root, "plugin", "agents")
		}
		return VendorSource{Release: release, Agents: agents, CoreTools: filepath.Join(root, "agent-core", "tools")}
	}
	if root := os.Getenv(SourceEnv); root != "" { //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
		return tree("source:"+root, root), nil
	}
	if rel, ok, err := runtime.Sibling(dir, nil); ok {
		return tree(rel.Tag, rel.Src), err
	}
	home := Home()
	release, err := os.ReadFile(filepath.Join(home, "release"))
	if err != nil {
		return VendorSource{}, fmt.Errorf("no scrum-master profiles installed at %s; install them with install.sh, or set %s", home, HomeEnv)
	}
	return VendorSource{Release: strings.TrimSpace(string(release)), Agents: filepath.Join(home, "agents"),
		CoreTools: filepath.Join(home, "agent-core", "tools")}, nil
}

// Source is the release an upgrade vendors from, as the vendor word
// resolves it: SourceEnv's tree, the sibling source checkout, else the
// release install.sh installed. The roll compares every registered
// repository's vendored release against it (GH-2813).
func Source(dir string) (VendorSource, error) { return vendorSource(dir) }

// CheckSource refuses, writing nothing, when the repository at dir has no
// release to vendor the profiles from (GH-49), so init and onboard can
// refuse before their first write rather than after.
func CheckSource(dir string) error {
	if _, err := vendorSource(dir); err != nil {
		return fmt.Errorf("%w; nothing was written", err)
	}
	return nil
}

// checkVendored is install --profiles --check: the vendored files that
// differ from the manifest, a failure when there are any (GH-35).
func checkVendored(ctx *cli.Context, dir string) error {
	var err error
	if dir == "" {
		if dir, err = gitRoot(); err != nil {
			return err
		}
	}
	findings, err := CheckVendored(dir)
	if err != nil {
		return err
	}
	if findings == nil {
		findings = []Finding{} // an empty report prints [], never null (GH-64)
	}
	var b strings.Builder
	for _, f := range findings {
		fmt.Fprintf(&b, "%-9s %s\n", f.Problem, rel(dir, f.Path))
	}
	if len(findings) == 0 {
		b.WriteString("the vendored profiles match the manifest")
	}
	if err := ctx.Emit(findings, b.String()); err != nil {
		return err
	}
	if len(findings) > 0 {
		return fmt.Errorf("%d vendored file(s) differ from %s; run the upgrade profile, or restore them", len(findings), ManifestFile)
	}
	return nil
}

// vendorProfiles is install --profiles: the vendoring, reported per file,
// the unchanged ones counted rather than listed.
func vendorProfiles(ctx *cli.Context, dir string, force bool) error {
	var err error
	if dir == "" {
		if dir, err = gitRoot(); err != nil {
			return err
		}
	}
	src, err := vendorSource(dir)
	if err != nil {
		return err
	}
	results, err := Vendor(dir, src, force, ctx.DryRun)
	if err != nil {
		return err
	}
	var b strings.Builder
	unchanged := 0
	for _, r := range results {
		if r.Action == "unchanged" {
			unchanged++
			continue
		}
		fmt.Fprintf(&b, "%-9s %s", r.Action, rel(dir, r.Path))
		if r.Reason != "" {
			fmt.Fprintf(&b, " (%s)", r.Reason)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "vendored %s into %s: %d files, %d unchanged", src.Release, VendorDir, len(results), unchanged)
	return ctx.Emit(struct {
		Release string   `json:"release"`
		Files   []Result `json:"files"`
	}{src.Release, results}, b.String())
}
