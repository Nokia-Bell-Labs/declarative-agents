// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package links

import (
	"bufio"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// The agile link lines an issue description may carry (srd007 R6.1,
// config format issue-body link_lines and implements_line).
const (
	BlockedBy  = "Blocked-by"
	Blocks     = "Blocks"
	DependsOn  = "Depends-on"
	RelatesTo  = "Relates-to"
	Duplicates = "Duplicates"
	Implements = "Implements"
)

// LinkKinds lists every agile link line, in the order issue-body declares.
var LinkKinds = []string{BlockedBy, Blocks, DependsOn, RelatesTo, Duplicates, Implements}

// Link is one agile link line.
type Link struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

var (
	issueRef = regexp.MustCompile(`^[A-Za-z0-9._/-]+#[0-9]+$`)
	docID    = regexp.MustCompile(`^(th|in|ep|ft|st|obj)[0-9]{3}-[a-z0-9-]+$`)
)

// ParseLinks returns every agile link line of a description, in order.
func ParseLinks(body string) []Link {
	var out []Link
	s := bufio.NewScanner(strings.NewReader(body))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		for _, k := range LinkKinds {
			if rest, ok := strings.CutPrefix(line, k+":"); ok {
				out = append(out, Link{Kind: k, Target: strings.TrimSpace(rest)})
				break
			}
		}
	}
	return out
}

// CheckTarget reports why a link's target is malformed: it must be an issue
// reference PROJECT#IID, a same-repository document id, or a URL pinned to a
// release tag (srd007 R6.1, R8.1). An Implements target names a story.
func CheckTarget(l Link) error {
	t := l.Target
	switch {
	case t == "":
		return fmt.Errorf("%s line names no target", l.Kind)
	case strings.Contains(t, "://"):
		if err := CheckPinned(t); err != nil {
			return err
		}
		if l.Kind == Implements {
			r, _ := Parse(t)
			if !strings.HasPrefix(path.Base(r.Path), "st") || path.Dir(r.Path) != "docs/specs/stories" {
				return fmt.Errorf("Implements names %s, not a story", t)
			}
		}
		return nil
	case docID.MatchString(t):
		if l.Kind == Implements && !strings.HasPrefix(t, "st") {
			return fmt.Errorf("Implements names %s, not a story", t)
		}
		return nil
	case issueRef.MatchString(t) && l.Kind != Implements:
		return nil
	}
	if l.Kind == Implements {
		return fmt.Errorf("Implements target %q is not a story id or a pinned story URL", t)
	}
	return fmt.Errorf("%s target %q is not PROJECT#IID, a document id, or a pinned URL", l.Kind, t)
}

// Edge is one blocking link: Waiting cannot proceed until On is done.
type Edge struct {
	Waiting string `json:"waiting"`
	On      string `json:"on"`
}

// BlockingEdges reads the blocking links of a set of issues, keyed by their
// references. A Blocks line on A naming B and a Blocked-by line on B naming A
// are one link (srd007 R6.2), so the result holds each edge once.
// Depends-on and Relates-to never block (R6.3).
func BlockingEdges(bodies map[string]string) []Edge {
	seen := map[Edge]bool{}
	var out []Edge
	add := func(e Edge) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	for ref, body := range bodies {
		for _, l := range ParseLinks(body) {
			switch l.Kind {
			case BlockedBy:
				add(Edge{Waiting: ref, On: l.Target})
			case Blocks:
				add(Edge{Waiting: l.Target, On: ref})
			}
		}
	}
	return out
}
