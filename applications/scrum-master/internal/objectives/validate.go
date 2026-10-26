// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package objectives

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
)

// Finding is one validation failure in one document.
type Finding struct {
	File    string `json:"file"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (f Finding) String() string { return fmt.Sprintf("%s: %s: %s", f.File, f.Field, f.Message) }

var (
	docIDPattern  = regexp.MustCompile(`^(in|obj)\d{3}-[a-z0-9-]+$`)
	groupPattern  = regexp.MustCompile(`^R(\d+)$`)
	itemPattern   = regexp.MustCompile(`^R(\d+)\.\d+$`)
	goalPattern   = regexp.MustCompile(`^G\d+$`)
	acPattern     = regexp.MustCompile(`^AC\d+$`)
	dueDateLayout = "2006-01-02"
)

// Validate checks one document against srd034 R1 and returns every finding.
func Validate(d *Document) []Finding {
	v := validator{file: d.Path}
	v.require("id", d.ID)
	v.require("title", d.Title)
	v.require("problem", d.Problem)
	v.require("owner", d.Owner)
	v.require("horizon", d.Horizon)
	if d.ID != "" {
		if !docIDPattern.MatchString(d.ID) {
			v.add("id", "%q is not inNNN-short-name or objNNN-short-name", d.ID)
		} else if want := strings.TrimSuffix(filepath.Base(d.Path), ".md"); d.Path != "" && d.ID != want {
			v.add("id", "%q does not match the file name %q", d.ID, want)
		}
	}
	switch {
	case d.Status == "":
		v.add("status", "missing")
	case !slices.Contains(Statuses, d.Status):
		v.add("status", "%q is not one of %s", d.Status, strings.Join(Statuses, ", "))
	}
	if len(d.Goals) == 0 {
		v.add("goals", "missing")
	}
	for _, g := range d.Goals {
		for id := range g {
			if !goalPattern.MatchString(id) {
				v.add("goals", "%q is not numbered Gn", id)
			}
		}
	}
	if len(d.NonGoals) == 0 {
		v.add("non_goals", "missing")
	}
	items := v.objectives(d)
	v.criteria(d, items)
	return v.findings
}

type validator struct {
	file     string
	findings []Finding
}

func (v *validator) add(field, format string, args ...any) {
	v.findings = append(v.findings, Finding{File: v.file, Field: field, Message: fmt.Sprintf(format, args...)})
}

func (v *validator) require(field, value string) {
	if strings.TrimSpace(value) == "" {
		v.add(field, "missing")
	}
}

// objectives checks every requirement group and returns the set of item ids.
func (v *validator) objectives(d *Document) map[string]bool {
	items := map[string]bool{}
	if len(d.Requirements) == 0 {
		v.add("requirements", "missing")
		return items
	}
	for _, gid := range d.ObjectiveIDs() {
		o := d.Requirements[gid]
		field := "requirements." + gid
		m := groupPattern.FindStringSubmatch(gid)
		if m == nil {
			v.add(field, "%q is not numbered Rn", gid)
			continue
		}
		v.require(field+".title", o.Title)
		if o.Due == "" {
			v.add(field+".due", "missing")
		} else if _, err := time.Parse(dueDateLayout, o.Due); err != nil {
			v.add(field+".due", "%q is not a YYYY-MM-DD date", o.Due)
		}
		for _, ref := range o.DeliveredBy {
			if err := links.CheckPinned(ref); err != nil {
				v.add(field+".delivered_by", "%v", err)
			}
		}
		if len(o.Items) == 0 {
			v.add(field+".items", "missing")
		}
		for _, it := range o.ItemsOf() {
			im := itemPattern.FindStringSubmatch(it.ID)
			switch {
			case im == nil || im[1] != m[1]:
				v.add(field+".items", "%q is not numbered %s.n", it.ID, gid)
			case items[it.ID]:
				v.add(field+".items", "%q appears twice", it.ID)
			case it.Text == "":
				v.add(field+".items", "%q has no text", it.ID)
			default:
				items[it.ID] = true
			}
		}
	}
	return items
}

func (v *validator) criteria(d *Document, items map[string]bool) {
	if len(d.AcceptanceCriteria) == 0 {
		v.add("acceptance_criteria", "missing")
		return
	}
	for i, c := range d.AcceptanceCriteria {
		field := fmt.Sprintf("acceptance_criteria[%d]", i)
		if !acPattern.MatchString(c.ID) {
			v.add(field+".id", "%q is not numbered ACn", c.ID)
		} else {
			field = "acceptance_criteria." + c.ID
		}
		v.require(field+".criterion", c.Criterion)
		if len(c.Tests) == 0 {
			v.add(field+".tests", "missing")
		}
		v.require(field+".environment", c.Environment)
		if len(c.Traces) == 0 {
			v.add(field+".traces", "missing")
		}
		for _, t := range c.Traces {
			if !items[t] {
				v.add(field+".traces", "%q names no item of this document", t)
			}
		}
	}
}
