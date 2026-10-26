// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/standup"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.IssueGroup)
}

// IssueGroup proves the issue group serves its subcommands, scrum-master program runs nothing and names its fix, and scrum-master issue recur opens the next instance once, finding one an earlier release standup recur opened (test-rel01.4-agent-legible-cli).
func (Integration) IssueGroup() error {
	r, err := newRigEnv()
	if err != nil {
		return err
	}
	po1, err := r.project("product-owners/po-1")
	if err != nil {
		return err
	}
	c := &check{}

	usage, err := r.runWords("issue", "--help")
	if err != nil {
		return err
	}
	for _, sub := range []string{"approve", "close", "file", "labels", "lint", "merge", "pop", "recur"} {
		c.expect(strings.Contains(string(usage), "\n  "+sub+" "), "scrum-master issue --help lists no %s:\n%s", sub, usage)
	}

	_, err = r.runWords("program", "pop", "--project", po1, "--issue", "1")
	c.expect(err != nil && strings.Contains(err.Error(), "scrum-master issue pop") && strings.Contains(err.Error(), "rerun the init or onboard profile"),
		"scrum-master program pop: %v, want a refusal naming scrum-master issue pop and the rerun that updates a skill", err)

	recurring := func(title, due, body string) (tracker.Issue, error) {
		return r.tr.CreateIssue(po1, tracker.NewIssue{Title: title, Labels: []string{"type::recurring"}, DueDate: due, Description: body})
	}
	recur := func(iid int) (standup.RecurResult, error) {
		var res standup.RecurResult
		return res, r.wordsJSON(&res, "issue", "recur", "--project", po1, "--issue", fmt.Sprint(iid))
	}
	fresh, err := recurring("Recurring: rig audit", "2026-10-15", "Audit the rig.\n\n## Previous Runs\n\n- none\n")
	if err != nil {
		return err
	}
	first, err := recur(fresh.IID)
	if err != nil {
		return err
	}
	again, err := recur(fresh.IID)
	if err != nil {
		return err
	}
	c.expect(!first.Existing && first.Next != 0 && again.Existing && again.Next == first.Next,
		"recur opened the next instance more than once: %+v then %+v", first, again)

	moved, err := recurring("Recurring: rig sweep", "2026-10-15", "Sweep the rig.\n\n## Previous Runs\n\n- none\n")
	if err != nil {
		return err
	}
	earlier, err := recurring("Recurring: rig sweep", "2026-10-22", fmt.Sprintf(
		"Sweep the rig.\n\n## Previous Runs\n\n- %s#%d (due 2026-10-15): closed by scrum-master standup recur without its work.\n\n<!-- scrum-master standup recurs %s#%d -->\n",
		po1, moved.IID, po1, moved.IID))
	if err != nil {
		return err
	}
	found, err := recur(moved.IID)
	if err != nil {
		return err
	}
	c.expect(found.Existing && found.Next == earlier.IID, "recur missed the instance an earlier release opened: %+v, want #%d", found, earlier.IID)
	return c.err("issueGroup")
}

// must returns out, dropping the error a later check reports.
func must(out []byte, _ error) []byte { return out }
