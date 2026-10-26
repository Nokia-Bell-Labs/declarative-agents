// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package links

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Cite is one "Cites: URL" line of an issue description (config format
// issue-body cites_line), with its optional " | review" or " | handoff" ask.
type Cite struct {
	URL string
	Ask string
}

// String renders the line.
func (c Cite) String() string {
	if c.Ask != "" {
		return "Cites: " + c.URL + " | " + c.Ask
	}
	return "Cites: " + c.URL
}

// ParseCites returns every Cites line of a description, in order.
func ParseCites(body string) []Cite {
	var cites []Cite
	s := bufio.NewScanner(strings.NewReader(body))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		rest, ok := strings.CutPrefix(line, "Cites:")
		if !ok {
			continue
		}
		u, ask, _ := strings.Cut(rest, "|")
		cites = append(cites, Cite{URL: strings.TrimSpace(u), Ask: strings.TrimSpace(ask)})
	}
	return cites
}

// Resolve reports why a pinned reference does not resolve, or nil when its
// tag exists, its path exists at the tag, and its anchor names an entry of
// the file there (srd007 R5.2). The reference must already be pinned.
func Resolve(t tracker.Forge, r Ref) error {
	if err := r.Pinned(); err != nil {
		return err
	}
	ok, err := t.TagExists(r.Project, r.Tag)
	if err != nil {
		return fmt.Errorf("%s: read tag %s: %w", r.Raw, r.Tag, err)
	}
	if !ok {
		return fmt.Errorf("%s: %s has no tag %q", r.Raw, r.Project, r.Tag)
	}
	content, err := t.GetFile(r.Project, r.Path, r.Tag)
	if errors.Is(err, tracker.ErrNotFound) {
		return fmt.Errorf("%s: %s does not exist at %s", r.Raw, r.Path, r.Tag)
	}
	if err != nil {
		return fmt.Errorf("%s: read %s at %s: %w", r.Raw, r.Path, r.Tag, err)
	}
	found, err := HasAnchor(r.Path, content, r.Anchor)
	if err != nil {
		return fmt.Errorf("%s: %s at %s does not parse: %w", r.Raw, r.Path, r.Tag, err)
	}
	if !found {
		return fmt.Errorf("%s: %s at %s has no entry %q", r.Raw, r.Path, r.Tag, r.Anchor)
	}
	return nil
}

// HasAnchor reports whether the Markdown document at path names anchor as a
// record id or a field: a requirement group (R2), an item (R2.1), or an
// interface name.
func HasAnchor(path string, content []byte, anchor string) (bool, error) {
	f, err := corpus.Decode(path, content)
	if err != nil {
		return false, err
	}
	return f.Markdown.HasKey(anchor), nil
}
