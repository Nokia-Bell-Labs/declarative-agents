// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// PushResult is one actionable signal and the issue that carries it.
type PushResult struct {
	Key    string `json:"key"`
	Action string `json:"action"` // filed, exists
	IID    int    `json:"iid,omitempty"`
	WebURL string `json:"web_url,omitempty"`
	// Admitted and Reasons are lint's result on the issue filed now; an
	// issue that already existed reports false with no reasons.
	Admitted bool     `json:"admitted"`
	Reasons  []string `json:"reasons,omitempty"`
}

func signalMarker(s Signal) string {
	return fmt.Sprintf("<!-- scrum-master reports signal=%s -->", s.Key())
}

// SignalIssue is the issue an actionable signal becomes, in do-work's input
// format (srd018 R6.3).
func SignalIssue(s Signal) tracker.NewIssue {
	body := fmt.Sprintf(`%s (%s, %s).

%s

## Requirements

1. Find the cause of the %s signal for %s and decide with the component's owner what changes.
2. Record the decision as a comment on this issue and update the affected Sprint Goal or blocker issues.

## Acceptance Criteria

- The next aggregate, which the aggregate skill runs, no longer reports %s for %s (test: reports_aggregate; environment: local)
`, s.Detail, s.Kind, s.Week, signalMarker(s), s.Kind, s.Subject, s.Kind, s.Subject)
	return tracker.NewIssue{Title: fmt.Sprintf("Signal: %s on %s", s.Kind, s.Subject), Description: body,
		Labels: append(program.Labels("management", "", "", s.Component), program.Op(program.OpReport))}
}

// signalComponent is the component a signal's issue carries: the signal's
// own when the program lists it or lists none, else the first component a
// shared blocker holds up.
func signalComponent(s Signal, p program.Policy) string {
	if !p.ComponentsRead || len(p.Components) == 0 || slices.Contains(p.Components, s.Component) || len(s.Blocked) == 0 {
		return s.Component
	}
	return s.Blocked[0]
}

// PushSignals files each actionable signal as an issue in the system
// repository, once: an open issue already carrying the signal is kept
// (srd018 R3.6, R6.3). A filed signal is linted in the same write and
// left unassigned, since only pop sets an assignee (srd041 R9.2): its body
// opens by mentioning the person pushing it, the program manager, as a
// status issue mentions its author. Its component is one the program
// lists, the first component a shared blocker holds up when the blocker
// itself is not one.
func PushSignals(t tracker.Tracker, system string, signals []Signal, p program.Policy, dryRun bool) ([]PushResult, error) {
	open, err := t.ListIssues(system, tracker.IssueQuery{State: "opened"})
	if err != nil {
		return nil, err
	}
	me, err := tracker.UserIn(t, system)
	if err != nil && !dryRun {
		return nil, err
	}
	var out []PushResult
	for _, s := range signals {
		if !s.Actionable {
			continue
		}
		r := PushResult{Key: s.Key(), Action: "filed"}
		for _, i := range open {
			if strings.Contains(i.Description, signalMarker(s)) {
				r.Action, r.IID, r.WebURL = "exists", i.IID, i.WebURL
				break
			}
		}
		if r.Action == "filed" {
			in := SignalIssue(s)
			if me.Username != "" {
				in.Description = "@" + me.Username + ": " + in.Description
			}
			if c := signalComponent(s, p); c != s.Component {
				in.Labels = slices.DeleteFunc(in.Labels, func(l string) bool { return strings.HasPrefix(l, "component::") })
				in.Labels = append(in.Labels, "component::"+c)
			}
			fr, err := program.File(t, system, in, p, dryRun)
			if err != nil {
				return out, err
			}
			r.IID, r.WebURL = fr.IID, fr.WebURL
			r.Admitted, r.Reasons = fr.Lint.Admitted, fr.Lint.Reasons
		}
		out = append(out, r)
	}
	return out, nil
}
