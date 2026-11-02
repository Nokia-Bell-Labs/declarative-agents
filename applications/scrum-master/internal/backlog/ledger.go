// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// LedgerEntry is one issue of a story's issues record (srd045 R4.4): who
// worked on it and when it opened and closed, kept in the story document
// so the record reads from the planning corpus whatever tracking system
// held the issue.
type LedgerEntry struct {
	Ref    string   `json:"ref"`
	URL    string   `json:"url,omitempty"`
	Title  string   `json:"title,omitempty"`
	Who    []string `json:"who,omitempty"`
	Opened string   `json:"opened,omitempty"`
	Closed string   `json:"closed,omitempty"`
	State  string   `json:"state,omitempty"`
}

// LedgerOf reads a story document's issues record, in ref order. A
// document without the field holds none: a story no issue has implemented
// yet, never an error.
func LedgerOf(doc []byte) ([]LedgerEntry, error) {
	md, err := corpus.ParseMarkdown(doc)
	if err != nil {
		return nil, err
	}
	tree, err := md.Field("issues").TreeErr()
	if err != nil {
		return nil, err
	}
	out := make([]LedgerEntry, 0, len(tree))
	for ref, v := range tree {
		e := LedgerEntry{Ref: ref}
		items, _ := v.([]any)
		for _, it := range items {
			m, _ := it.(map[string]any)
			for k, val := range m {
				s := strings.TrimSpace(fmt.Sprint(val))
				switch k {
				case "url":
					e.URL = s
				case "title":
					e.Title = s
				case "who":
					for _, w := range strings.Split(s, ",") {
						if w = strings.TrimSpace(w); w != "" {
							e.Who = append(e.Who, w)
						}
					}
				case "opened":
					e.Opened = s
				case "closed":
					e.Closed = s
				case "state":
					e.State = s
				}
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, nil
}

// EntryFor is the record of one tracker issue: who is who works on it —
// its assignees, nobody for an open issue none has picked up, its author
// for a closed one with none, who worked it (GH-2855); the dates are days,
// not instants; and a tracker's "opened" state reads as open.
func EntryFor(project string, i tracker.Issue) LedgerEntry {
	state := i.State
	if state == "" || state == "opened" {
		state = "open"
	}
	if i.ClosedAt != "" {
		state = "closed"
	}
	who := append([]string(nil), i.Assignees...)
	if len(who) == 0 && state == "closed" && i.Author != "" {
		who = []string{i.Author}
	}
	return LedgerEntry{
		Ref:    fmt.Sprintf("%s#%d", project, i.IID),
		URL:    i.WebURL,
		Title:  i.Title,
		Who:    who,
		Opened: day(i.CreatedAt),
		Closed: day(i.ClosedAt),
		State:  state,
	}
}

// day is an instant's day, as the record keeps days.
func day(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// ledgerMarker opens the issues field as the record writes it.
const ledgerMarker = `<!-- field: issues | form: "tree" -->`

// WriteLedger returns the story document with its issues field holding
// exactly entries, in ref order: the field rewritten in place where the
// document holds one, appended at its end where it does not, and removed
// when entries is empty, since an absent field is a story no issue has
// implemented yet. Writing the same entries twice writes the same bytes.
func WriteLedger(doc []byte, entries []LedgerEntry) []byte {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Ref < entries[j].Ref })
	var b strings.Builder
	if len(entries) > 0 {
		b.WriteString(ledgerMarker + "\n## Issues\n\n")
		for _, e := range entries {
			fmt.Fprintf(&b, "- %q:\n", e.Ref)
			if e.URL != "" {
				fmt.Fprintf(&b, "  - url: %q\n", e.URL)
			}
			if e.Title != "" {
				fmt.Fprintf(&b, "  - title: %q\n", e.Title)
			}
			if len(e.Who) > 0 {
				fmt.Fprintf(&b, "  - who: %s\n", strings.Join(e.Who, ", "))
			}
			if e.Opened != "" {
				fmt.Fprintf(&b, "  - opened: %q\n", e.Opened)
			}
			if e.Closed != "" {
				fmt.Fprintf(&b, "  - closed: %q\n", e.Closed)
			}
			if e.State != "" {
				fmt.Fprintf(&b, "  - state: %s\n", e.State)
			}
		}
	}
	section := b.String()
	start := bytes.Index(doc, []byte("<!-- field: issues"))
	if start < 0 {
		if section == "" {
			return doc
		}
		return append(append(bytes.TrimRight(doc, "\n"), '\n', '\n'), section...)
	}
	end := len(doc)
	if next := bytes.Index(doc[start+1:], []byte("\n<!-- ")); next >= 0 {
		end = start + 1 + next + 1 // keep the newline before the next marker
	}
	rest := doc[end:]
	head := bytes.TrimRight(doc[:start], "\n")
	var out []byte
	out = append(out, head...)
	if section != "" {
		out = append(out, '\n', '\n')
		out = append(out, section...)
	} else if len(rest) > 0 {
		out = append(out, '\n')
	}
	if len(rest) > 0 {
		out = append(out, '\n')
		out = append(out, rest...)
	} else if section != "" || len(head) > 0 {
		out = bytes.TrimRight(out, "\n")
		out = append(out, '\n')
	}
	return out
}
