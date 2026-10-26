// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.InitProfile)
}

// tpmWord is tpm as a word, the retired binary's name, in either case, or
// a TPM_ variable, which GH-38 renamed SCRUM_MASTER_; tool names such as
// a_tpm_b are not it.
var tpmWord = regexp.MustCompile(`(?i)(^|[^a-z0-9_])tpm([^a-z0-9_]|$)|\bTPM_`)

// InitProfile runs the repository's agent over the init profile in an empty
// git repository with an empty tracker beside it and no scrum-master on PATH
// (GH-29): with no profiles installed to vendor, it refuses and writes
// nothing (GH-49); with them, the run ends in Done with the corpus, the seeded tracker, the
// skills of the requested agent, and the profiles vendored into
// .scrum-master; every skill runs its vendored profile with the agent, the
// vendored profiles validate against the vendored core root, a second run
// from the vendored init profile changes nothing, and the vendored check
// profile passes, then names an edited vendored profile (test: init profile
// run, check profile run; environment: local).
func (Integration) InitProfile() error {
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "scrum-master-init-profile-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return err
	}
	repo, tracker := filepath.Join(tmp, "system"), filepath.Join(tmp, "tracker.git")
	env := append(os.Environ(), "PATH="+pathWithout("tpm", "scrum-master"), install.SourceEnv+"="+rel.Src, "SCRUM_MASTER_USER=operator",
		"GIT_AUTHOR_NAME=operator", "GIT_AUTHOR_EMAIL=operator@example.invalid",
		"GIT_COMMITTER_NAME=operator", "GIT_COMMITTER_EMAIL=operator@example.invalid")
	sh := func(dir, name string, args ...string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out)
		}
		return string(out), nil
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main", repo}, {"init", "-q", "--bare", "-b", "main", tracker}} {
		if _, err := sh(tmp, "git", args...); err != nil {
			return err
		}
	}
	request := filepath.Join(tmp, "request.json")
	req, _ := json.Marshal(map[string]any{"parameters": map[string]string{"agent": "claude", "tracker": tracker, "group": "rig"}})
	if err := os.WriteFile(request, req, 0o644); err != nil {
		return err
	}
	run := func(profile, coreRoot, out string) (string, error) {
		cmd := exec.Command(rel.Agent, "--profile", profile, "--core-root", coreRoot, "--directory", repo,
			"--output", filepath.Join(tmp, out), "--request", request)
		cmd.Dir, cmd.Env = repo, env
		var b bytes.Buffer
		cmd.Stdout, cmd.Stderr = &b, &b
		err := cmd.Run()
		return b.String(), err
	}
	c := &check{}

	// With no profiles to vendor, init refuses before its first write
	// (GH-49): the repository and the tracker stay empty.
	bare, empty := filepath.Join(tmp, "unvendored"), filepath.Join(tmp, "no-profiles")
	if _, err := sh(tmp, "git", "init", "-q", "-b", "main", bare); err != nil {
		return err
	}
	refuse := exec.Command(rel.Agent, "--profile", filepath.Join(rel.Src, "applications", "scrum-master", "plugin", "agents", "init", "profile.yaml"),
		"--core-root", rel.CoreRoot(), "--directory", bare, "--output", filepath.Join(tmp, "unvendored-run"), "--request", request)
	refuse.Dir = bare
	for _, kv := range env {
		if !strings.HasPrefix(kv, install.SourceEnv+"=") {
			refuse.Env = append(refuse.Env, kv)
		}
	}
	refuse.Env = append(refuse.Env, install.HomeEnv+"="+empty)
	refused, rerr := refuse.CombinedOutput()
	written, _ := os.ReadDir(bare)
	c.expect(rerr != nil && strings.Contains(string(refused), "final machine state: Refused") && strings.Contains(string(refused), "nothing was written"),
		"init without profiles must refuse before writing: %v: %s", rerr, tail17(string(refused)))
	c.expect(len(written) == 1, "init without profiles wrote into the repository: %v", written)
	_, seeded := exec.Command("git", "--git-dir", tracker, "rev-parse", "--verify", "-q", "main").Output()
	c.expect(seeded != nil, "init without profiles seeded the tracker")

	out, runErr := run(filepath.Join(rel.Src, "applications", "scrum-master", "plugin", "agents", "init", "profile.yaml"), rel.CoreRoot(), "first")
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the init run must end in Done: %v: %s", runErr, tail17(out))
	for _, p := range []string{"AGENTS.md", "README.md", "docs/VISION.md", "control/policy.md", install.ManifestFile,
		install.AgentsVendored + "/pop/profile.yaml", install.CoreVendored + "/tools/builtin/done.yaml"} {
		_, err := os.Stat(filepath.Join(repo, p))
		c.expect(err == nil, "the init run wrote no %s: %v", p, err)
	}
	_, cursor := os.Stat(filepath.Join(repo, ".cursor"))
	c.expect(os.IsNotExist(cursor), "the request named claude, but cursor's skills were installed")
	skill, err := os.ReadFile(filepath.Join(repo, ".claude", "skills", "pop", "SKILL.md"))
	c.expect(err == nil && strings.Contains(string(skill), install.RunCommand("pop")), "pop's skill does not run its vendored profile: %v\n%s", err, skill)
	config, err := exec.Command("git", "--git-dir", tracker, "show", "main:.beads/config.yaml").Output()
	c.expect(err == nil && strings.Contains(string(config), "issue-prefix: rig"), "the tracker was not seeded: %q %v", config, err)
	agents, _ := filepath.Glob(filepath.Join(repo, install.AgentsVendored, "*", "profile.yaml"))
	c.expect(len(agents) > 0, "no profile was vendored")
	for _, p := range agents {
		cmd := exec.Command(rel.Agent, "--profile", p, "--core-root", filepath.Join(repo, install.CoreVendored), "--validate-config")
		cmd.Env = env
		o, err := cmd.CombinedOutput()
		c.expect(err == nil, "%s does not validate against the vendored core root: %v: %s", p, err, o)
	}

	// Nothing the run wrote names tpm, the binary the words no longer have
	// (GH-36).
	named := 0
	filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.IsDir():
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && tpmWord.Match(b) {
			named++
			c.expect(false, "%s names tpm", p)
		}
		return nil
	})
	c.expect(named == 0, "%d written files name tpm", named)
	if _, err := sh(repo, "git", "add", "-A"); err != nil {
		return err
	}
	if _, err := sh(repo, "git", "commit", "-q", "-m", "Lay down the program"); err != nil {
		return err
	}
	out, runErr = run(filepath.Join(repo, install.AgentsVendored, "init", "profile.yaml"), filepath.Join(repo, install.CoreVendored), "second")
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the rerun from the vendored profile must end in Done: %v: %s", runErr, tail17(out))
	status, err := sh(repo, "git", "status", "--porcelain")
	c.expect(err == nil && strings.TrimSpace(status) == "", "the rerun changed the repository: %s %v", status, err)

	// The check CI runs (GH-35): it passes on the repository as laid down,
	// and an edited vendored profile ends it in VendoredDrifted.
	check := filepath.Join(repo, install.AgentsVendored, "check", "profile.yaml")
	core := filepath.Join(repo, install.CoreVendored)
	out, runErr = run(check, core, "check")
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the check must pass on the laid-down repository: %v: %s", runErr, tail17(out))
	edited := filepath.Join(repo, install.AgentsVendored, "pop", "profile.yaml")
	if err := os.WriteFile(edited, []byte("name: pop # edited\n"), 0o644); err != nil {
		return err
	}
	out, runErr = run(check, core, "check-drifted")
	c.expect(runErr != nil && strings.Contains(out, "final machine state: VendoredDrifted") && strings.Contains(out, "pop/profile.yaml"),
		"an edited vendored profile must end the check in VendoredDrifted, naming it: %v: %s", runErr, tail17(out))
	return c.err("initProfile")
}

func init() {
	integrationTargets = append(integrationTargets, Integration{}.OnboardProfile)
}

// OnboardProfile runs the repository's agent over the onboard profile in a
// joining repository of the rig's program, with no scrum-master on PATH (GH-29): the
// run ends in Done with the registration filed in the system repository,
// the skills installed, and the profiles vendored, and a second run files
// no second registration (test: onboard profile run; environment: local).
func (Integration) OnboardProfile() error {
	r, err := newRigEnv()
	if err != nil {
		return err
	}
	system, err := r.project("system")
	if err != nil {
		return err
	}
	po1, err := r.project("product-owners/po-1")
	if err != nil {
		return err
	}
	po2, err := r.project("product-owners/po-2")
	if err != nil {
		return err
	}
	if err := r.seedSystem(system, po1, po2); err != nil {
		return err
	}
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	// Beside the rig's checkouts, where the tracker finds the system
	// repository as a sibling of the checkout it runs from; named apart
	// from the onboard target's repository, which shares the rig.
	store := filepath.Join(r.run.Dir, "checkouts", "store-profile")
	if err := os.MkdirAll(store, 0o755); err != nil {
		return err
	}
	if err := gitIn(store, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	request := filepath.Join(r.run.Dir, "onboard-request.json")
	req, _ := json.Marshal(map[string]any{"parameters": map[string]string{
		"agent": "claude", "ref": seedBranch, "repository": r.webBase + "/rig/product-owners/store-profile"}})
	if err := os.WriteFile(request, req, 0o644); err != nil {
		return err
	}
	run := func(out string) (string, error) {
		cmd := exec.Command(rel.Agent,
			"--profile", filepath.Join(rel.Src, "applications", "scrum-master", "plugin", "agents", "onboard", "profile.yaml"),
			"--core-root", rel.CoreRoot(), "--directory", store, "--output", filepath.Join(r.run.Dir, out), "--request", request)
		cmd.Dir = store
		cmd.Env = append(os.Environ(),
			"PATH="+pathWithout("tpm", "scrum-master"),
			install.SourceEnv+"="+rel.Src,
			"SCRUM_MASTER_TRACKER="+filepath.Join(r.run.Dir, "tracker.git"),
			"SCRUM_MASTER_TRACKER_CLONE="+filepath.Join(r.run.Dir, "tracker-"+r.user),
			"SCRUM_MASTER_USER="+r.user,
			"SCRUM_MASTER_WEB_BASE="+r.webBase,
			"SCRUM_MASTER_SYSTEM="+system,
			"SCRUM_MASTER_SUBSYSTEM=srd010-po-1",
		)
		var b bytes.Buffer
		cmd.Stdout, cmd.Stderr = &b, &b
		err := cmd.Run()
		return b.String(), err
	}
	registrations := func() int {
		n := 0
		issues, err := r.tr.ListIssues(system, tracker.IssueQuery{State: "all"})
		if err != nil {
			return -1
		}
		for _, i := range issues {
			if strings.Contains(i.Title, "rig/product-owners/store-profile") {
				n++
			}
		}
		return n
	}
	c := &check{}
	out, runErr := run("onboard-first")
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the onboard run must end in Done: %v: %s", runErr, tail17(out))
	c.expect(registrations() == 1, "registrations after the first run: %d, want 1", registrations())
	for _, p := range []string{"AGENTS.md", install.ManifestFile, install.AgentsVendored + "/pop/profile.yaml", ".claude/skills/pop/SKILL.md"} {
		_, err := os.Stat(filepath.Join(store, p))
		c.expect(err == nil, "the onboard run wrote no %s: %v", p, err)
	}
	out, runErr = run("onboard-second")
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the second onboard run must end in Done: %v: %s", runErr, tail17(out))
	c.expect(registrations() == 1, "registrations after the second run: %d, want 1", registrations())
	return c.err("onboardProfile")
}
