// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// An admission rule is one named check an issue passes to be admitted
// (srd013 R1.1; config format policy, admission.rules). The policy names
// the rules that apply, so a program disables one by removing its name;
// the checks admission cannot express as named rules — pinned Cites and
// the link-line discipline (srd007) — always apply and the policy format
// lists them as fixed.
type rule struct {
	name  string
	check func(tracker.Issue, Policy) []string
}

// rules are the named admission checks in their order; a policy without an
// admission.rules field applies them all.
var rules = []rule{
	{"one-plane", onePlane},
	{"one-type", oneType},
	{"one-component", oneComponent},
	{"one-op", oneOp},
	{"kind-on-data", kindOnData},
	{"data-milestone", dataMilestone},
	{"work-sections", workSections},
	{"management-filers", managementFilers},
}

// RuleNames are the named admission rules in their order, what the default
// policy applies and control/policy.md lists.
func RuleNames() []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.name
	}
	return out
}

// axis is the values of one label axis on an issue.
func axis(i tracker.Issue, prefix string) []string {
	var vals []string
	for _, l := range i.Labels {
		if v, ok := strings.CutPrefix(l, prefix); ok {
			vals = append(vals, v)
		}
	}
	return vals
}

// listed is the reasons vals holds values allowed does not list; an empty
// allowed accepts anything.
func listed(name string, vals, allowed []string) []string {
	var out []string
	for _, v := range vals {
		if len(allowed) > 0 && !slices.Contains(allowed, v) {
			out = append(out, fmt.Sprintf("%s::%s is not a listed %s", name, v, name))
		}
	}
	return out
}

func onePlane(i tracker.Issue, p Policy) []string {
	planes := axis(i, "plane::")
	var out []string
	if len(planes) != 1 {
		out = append(out, fmt.Sprintf("needs exactly one plane:: label, has %d", len(planes)))
	}
	return append(out, listed("plane", planes, p.Planes)...)
}

func oneType(i tracker.Issue, p Policy) []string {
	types := canonicalTypes(axis(i, "type::"))
	var out []string
	if len(types) > 1 {
		out = append(out, fmt.Sprintf("has %d type:: labels, at most one", len(types)))
	}
	return append(out, listed("type", types, p.Types)...)
}

func oneComponent(i tracker.Issue, p Policy) []string {
	components := axis(i, "component::")
	sprint := slices.Contains(canonicalTypes(axis(i, "type::")), "sprint")
	bootstrap := p.ComponentsRead && len(p.Components) == 0 && len(components) == 0
	var out []string
	if !sprint && !bootstrap && len(components) != 1 {
		out = append(out, fmt.Sprintf("needs exactly one component:: label, has %d", len(components)))
	}
	return append(out, listed("component", components, p.Components)...)
}

func oneOp(i tracker.Issue, _ Policy) []string {
	if ops := axis(i, "op::"); len(ops) > 1 {
		return []string{fmt.Sprintf("has %d op:: labels, at most one", len(ops))}
	}
	return nil
}

func kindOnData(i tracker.Issue, p Policy) []string {
	kinds := axis(i, "kind::")
	data := slices.Contains(axis(i, "plane::"), "data")
	types := canonicalTypes(axis(i, "type::"))
	var out []string
	if data && len(types) == 0 && len(kinds) != 1 {
		out = append(out, fmt.Sprintf("a data issue needs exactly one kind:: label, has %d", len(kinds)))
	}
	if !data && len(kinds) > 0 {
		out = append(out, "only data issues carry a kind:: label")
	}
	return append(out, listed("kind", kinds, p.Kinds)...)
}

func dataMilestone(i tracker.Issue, p Policy) []string {
	data := slices.Contains(axis(i, "plane::"), "data")
	types := canonicalTypes(axis(i, "type::"))
	if data && len(types) == 0 && p.RequireMilestone && i.Milestone == "" {
		return []string{"a data issue needs a sprint milestone"}
	}
	return nil
}

func workSections(i tracker.Issue, p Policy) []string {
	needAC := len(links.ParseCites(i.Description)) > 0
	for _, k := range axis(i, "kind::") {
		needAC = needAC || slices.Contains(p.RequireAC, k)
	}
	reqs, haveReqs := section(i.Description, "Requirements")
	criteria, haveAC := section(i.Description, "Acceptance Criteria")
	var out []string
	if needAC && (!haveReqs || len(reqs) == 0) {
		out = append(out, "needs a ## Requirements section with at least one entry")
	}
	if needAC && (!haveAC || len(criteria) == 0) {
		out = append(out, "needs a ## Acceptance Criteria section with at least one entry")
	}
	for _, c := range criteria {
		if !evidencePattern.MatchString(c) {
			out = append(out, fmt.Sprintf("criterion %q names no (test: T; environment: E)", shorten(c)))
		}
	}
	return out
}

func managementFilers(i tracker.Issue, p Policy) []string {
	planes := axis(i, "plane::")
	if slices.Contains(planes, "management") || slices.Contains(planes, "control") {
		if len(p.ManagementFilers) > 0 && p.ManagementFilers[0] != "leads" && !slices.Contains(p.ManagementFilers, i.Author) {
			return []string{fmt.Sprintf("%s may not file a %s issue", i.Author, planes[0])}
		}
	}
	return nil
}
