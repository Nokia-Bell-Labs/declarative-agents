// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/scaffold"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.Onboard)
}

// Onboard onboards a new repository into the rig's program: its corpus passes the parent's corpus audit as written, the orientation files are written for the subsystem level with scrum-master's block appended to the README.md and AGENTS.md it already had, the system repository gets one registration issue and no commit, and a second onboard files none (test-rel01.4-agent-legible-cli, test-rel04.1-orientation-doctor).
func (Integration) Onboard() error {
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
	store := filepath.Join(r.run.Dir, "joining", "store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		return err
	}
	if err := gitIn(store, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	// The joining repository already has its own README.md and AGENTS.md;
	// onboard appends scrum-master's block after their text (srd043 R3.5).
	own := map[string]string{"README.md": "# store\n\nThe team's own README.\n", scaffold.AgentsFile: "Run the linter before committing.\n"}
	for f, text := range own {
		if err := os.WriteFile(filepath.Join(store, f), []byte(text), 0o644); err != nil {
			return err
		}
	}
	head := func() (string, error) {
		return gitOut(r.run.Dir, "--git-dir", r.origin(system), "rev-parse", "refs/heads/main")
	}
	before, err := head()
	if err != nil {
		return err
	}
	onboard := func() (scaffold.OnboardResult, error) {
		var res scaffold.OnboardResult
		return res, r.wordsJSON(&res, "onboard", "--dir", store, "--system", system, "--ref", seedBranch,
			"--subsystem", "srd010-po-1", "--repository", r.webBase+"/rig/product-owners/store", "--agent", "claude")
	}
	c := &check{}
	first, err := onboard()
	if err != nil {
		return err
	}
	c.expect(first.Registration.Filed && first.Registration.Issue != nil, "no registration filed: %+v", first.Registration)
	if first.Registration.Issue != nil {
		i, err := r.issue(system, first.Registration.Issue.IID)
		c.expect(err == nil && strings.Contains(i.Title, "rig/product-owners/store") && strings.Contains(i.Description, "srd010-po-1"),
			"registration issue: %+v %v", i, err)
	}

	expectOrientation(c, store, []string{"epics", "features", "stories"}, []string{"themes", "initiatives"})
	for f, text := range own {
		b, _ := os.ReadFile(filepath.Join(store, f))
		action := ""
		for _, res := range first.Files {
			if res.Path == f {
				action = res.Action
			}
		}
		c.expect(action == "appended" && strings.HasPrefix(string(b), text+"\n<!-- managed:begin"), "%s: %s, the team's text not first:\n%s", f, action, b)
	}
	if err := gitIn(store, "add", "-A"); err != nil {
		return err
	}
	c.expect(runCorpusAudit(store, runStreaming) == nil, "the parent's corpus audit fails on the corpus scrum-master onboard wrote")
	c.expect(specCheckAt(store) == nil, "the spec tool's check fails on the corpus scrum-master onboard wrote")

	second, err := onboard()
	if err != nil {
		return err
	}
	c.expect(!second.Registration.Filed && strings.Contains(second.Registration.Reason, "already asks"),
		"second onboard: %+v", second.Registration)
	for _, f := range second.Files {
		c.expect(f.Action == "kept", "second onboard: %s %s", f.Path, f.Action)
	}
	open, err := r.issues(system)
	if err != nil {
		return err
	}
	registrations := 0
	for _, i := range open {
		// Its own repository's: another target in the same rig may
		// register one of its own (onboardProfile, GH-29).
		if strings.Contains(i.Description, "scrum-master onboard registers") && strings.Contains(i.Title, "rig/product-owners/store") &&
			!strings.Contains(i.Title, "rig/product-owners/store-profile") {
			registrations++
		}
	}
	c.expect(registrations == 1, "%d registration issues in the system repository, want 1", registrations)
	after, err := head()
	c.expect(err == nil && after == before, "onboard committed to the system repository: %s, was %s (%v)", after, before, err)
	return c.err("onboard")
}
