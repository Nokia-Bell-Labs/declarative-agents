// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
)

// The srd041 states.
const (
	Open        = "open"
	NeedsTriage = "needs-triage"
	Ready       = "ready"
	Taken       = "taken"
	InProgress  = "in-progress"
	Closed      = "closed"
)

// The srd041 overlays, held as labels. A row's To is "+overlay" to set one
// and "-overlay" to clear it.
const (
	Waiting  = "waiting"
	Blocked  = "blocked"
	NeedsRun = "needs-run"
)

// The srd041 actors.
const (
	ActorPerson     = "person"
	ActorPop        = "pop"
	ActorMerge      = "merge"
	ActorTrackGoals = "track-goals"
	ActorHandoff    = "handoff"
	ActorLint       = "lint"
)

// The srd041 assignee effects.
const (
	EffectNone  = "none"
	EffectSet   = "set"
	EffectClear = "clear"
)

// KindSprintGoal restricts a row to Sprint Goal issues, objective issues
// filed before release 04.0 among them (srd034 R3.7).
const KindSprintGoal = "sprint-goal"

// ErrRefused is wrapped by every transition the table does not list.
var ErrRefused = errors.New("refused")

// StatesFile is the config document holding a program's transitions table
// (config format states): scrum-master init writes the default, and the tracker
// reads the program's own rows from it, overlay rows among them.
const StatesFile = "control/states.md"

// Row is one row of the srd041 transitions table.
type Row struct {
	From     []string
	To       string
	Actor    string
	Assignee string
	// Requires is a label the issue must carry for the row to apply.
	Requires string
	// Kind restricts the row to a kind of issue.
	Kind string
}

// AnyOpen is every state but closed: the any-open shorthand of the states
// config format.
var AnyOpen = []string{Open, NeedsTriage, Ready, Taken, InProgress}

var anyOpen = AnyOpen

// Table is the default srd041 state machine (R8.1), the rows scrum-master embeds
// and init writes to control/states.md: every transition the tracker makes
// is one of a program's rows, fired by the row's one actor.
var Table = []Row{
	{From: []string{Open}, To: NeedsTriage, Actor: ActorLint, Assignee: EffectNone},
	{From: []string{Open}, To: Ready, Actor: ActorLint, Assignee: EffectNone},
	{From: []string{Open}, To: Taken, Actor: ActorPop, Assignee: EffectSet},
	{From: []string{NeedsTriage}, To: Ready, Actor: ActorLint, Assignee: EffectNone},
	{From: []string{NeedsTriage}, To: Taken, Actor: ActorLint, Assignee: EffectNone},
	{From: []string{Ready}, To: Taken, Actor: ActorPop, Assignee: EffectSet},
	{From: []string{Taken}, To: Ready, Actor: ActorPerson, Assignee: EffectClear},
	{From: []string{Taken}, To: InProgress, Actor: ActorPerson, Assignee: EffectNone},
	{From: []string{InProgress}, To: Taken, Actor: ActorPerson, Assignee: EffectNone},
	{From: anyOpen, To: "+" + Waiting, Actor: ActorTrackGoals, Assignee: EffectNone},
	{From: anyOpen, To: "-" + Waiting, Actor: ActorTrackGoals, Assignee: EffectNone},
	{From: []string{Taken, InProgress}, To: "+" + Blocked, Actor: ActorPerson, Assignee: EffectNone},
	{From: []string{Taken, InProgress}, To: "-" + Blocked, Actor: ActorPerson, Assignee: EffectNone},
	{From: anyOpen, To: "+" + NeedsRun, Actor: ActorHandoff, Assignee: EffectNone, Requires: "type::recurring"},
	{From: anyOpen, To: Closed, Actor: ActorMerge, Assignee: EffectNone},
	{From: anyOpen, To: Closed, Actor: ActorTrackGoals, Assignee: EffectNone, Kind: KindSprintGoal},
	{From: anyOpen, To: Closed, Actor: ActorPerson, Assignee: EffectNone},
	{From: []string{Closed}, To: Open, Actor: ActorPerson, Assignee: EffectNone},
}

// statesDoc is what ParseStates reads of a states config document: each
// table row a mapping keyed by the header (design.md D11).
type statesDoc struct {
	Rows []struct {
		From     string `yaml:"from"`
		To       string `yaml:"to"`
		Actor    string `yaml:"actor"`
		Assignee string `yaml:"assignee"`
		Requires string `yaml:"requires"`
		Kind     string `yaml:"kind"`
	} `yaml:"rows"`
}

var (
	actors    = []string{ActorPerson, ActorPop, ActorMerge, ActorTrackGoals, ActorHandoff, ActorLint}
	effects   = []string{EffectNone, EffectSet, EffectClear}
	allStates = []string{Open, NeedsTriage, Ready, Taken, InProgress, Closed}
)

// ParseStates decodes a states config document into transitions rows
// (srd041 R8). A row it cannot hold to the format — an unknown state,
// actor, assignee effect, or kind — refuses the whole document, so a typo
// never silently drops a rule.
func ParseStates(b []byte) ([]Row, error) {
	var doc statesDoc
	if err := corpus.DecodeConfig(b, &doc); err != nil {
		return nil, err
	}
	if len(doc.Rows) == 0 {
		return nil, errors.New("the states document holds no rows")
	}
	out := make([]Row, 0, len(doc.Rows))
	for n, r := range doc.Rows {
		refuse := func(what, v string) error {
			return fmt.Errorf("rows[%d]: %s %q is not one the states format lists", n+1, what, v)
		}
		row := Row{To: r.To, Actor: r.Actor, Assignee: r.Assignee, Requires: r.Requires, Kind: r.Kind}
		for _, f := range strings.Split(r.From, ",") {
			switch f = strings.TrimSpace(f); {
			case f == "any-open":
				row.From = append(row.From, AnyOpen...)
			case slices.Contains(allStates, f):
				row.From = append(row.From, f)
			default:
				return nil, refuse("from state", f)
			}
		}
		if len(row.From) == 0 {
			return nil, fmt.Errorf("rows[%d]: from is empty", n+1)
		}
		overlay := strings.TrimLeft(row.To, "+-")
		switch {
		case overlay != row.To && overlay == "":
			return nil, refuse("to overlay", row.To)
		case overlay == row.To && !slices.Contains(allStates, row.To):
			return nil, refuse("to state", row.To)
		}
		if !slices.Contains(actors, row.Actor) {
			return nil, refuse("actor", row.Actor)
		}
		if row.Assignee == "" {
			row.Assignee = EffectNone
		}
		if !slices.Contains(effects, row.Assignee) {
			return nil, refuse("assignee effect", row.Assignee)
		}
		if row.Kind != "" && row.Kind != KindSprintGoal {
			return nil, refuse("kind", row.Kind)
		}
		out = append(out, row)
	}
	return out, nil
}

func isSprintGoal(i Issue) bool {
	return i.HasLabel("plane::management") && sprintgoal.Is(i.Description)
}

// row finds the row of rows that lets actor move i to to.
func row(rows []Row, i Issue, actor, to string) (Row, bool) {
	for _, r := range rows {
		if r.Actor != actor || r.To != to || !slices.Contains(r.From, i.State) {
			continue
		}
		if r.Requires != "" && !i.HasLabel(r.Requires) {
			continue
		}
		if r.Kind == KindSprintGoal && !isSprintGoal(i) {
			continue
		}
		if overlay, ok := strings.CutPrefix(to, "-"); ok && !i.HasLabel(overlay) {
			continue
		}
		return r, true
	}
	return Row{}, false
}

// transition fires actor's move of i to to on behalf of caller, or refuses
// with the attempted transition named, against the store's rows — the
// program's control/states.md, else the embedded default (srd041 R8). Only
// pop sets an assignee, and only the caller (R9.1); a person's moves on a
// taken issue are the assignee's alone (R9.2).
func (s *Store) transition(i *Issue, actor, to, caller string) error {
	refuse := func(why string) error {
		return fmt.Errorf("%w: %s %s to %s by %s%s (srd041)", ErrRefused, i.ID, i.State, to, actor, why)
	}
	rows, err := s.states()
	if err != nil {
		return err
	}
	r, ok := row(rows, *i, actor, to)
	if !ok {
		return refuse("")
	}
	if actor == ActorPerson && to != Closed && to != Open && i.Assignee != "" && i.Assignee != caller {
		return refuse(", which only the assignee " + i.Assignee + " may fire")
	}
	if r.Assignee == EffectSet && i.Assignee != "" && i.Assignee != caller {
		return refuse(", taken by " + i.Assignee)
	}

	if overlay, ok := strings.CutPrefix(to, "+"); ok {
		if !i.HasLabel(overlay) {
			i.Labels = append(i.Labels, overlay)
		}
		return nil
	}
	if overlay, ok := strings.CutPrefix(to, "-"); ok {
		i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == overlay })
		return nil
	}

	switch r.Assignee {
	case EffectSet:
		i.Assignee = caller
	case EffectClear:
		i.Assignee = ""
	}
	i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == NeedsTriage })
	if to == NeedsTriage {
		i.Labels = append(i.Labels, NeedsTriage)
	}
	i.State, i.StateActor = to, actor
	switch to {
	case Closed:
		i.Status = StatusClosed
	case InProgress:
		i.Status = StatusInProgress
	default:
		i.Status = StatusOpen
		i.ClosedAt, i.CloseReason = "", ""
	}
	return nil
}
