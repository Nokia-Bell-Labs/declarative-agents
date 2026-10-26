// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package links parses and checks cross-repository references (srd007 R5).
package links

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Ref is a cross-repository reference of the form
// https://HOST/GROUP/REPO/-/blob/TAG/PATH#ANCHOR (srd007 R5.1).
type Ref struct {
	Raw     string
	Host    string
	Project string // GROUP/REPO, possibly nested
	Tag     string
	Path    string
	Anchor  string
}

// branchNames are refs that move; a reference to one is never pinned (srd007 R5.3).
var branchNames = map[string]bool{"main": true, "master": true, "develop": true, "HEAD": true, "trunk": true}

var commitSHA = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// Parse splits a reference URL. It reports a malformed URL but does not judge
// whether the reference is pinned; Pinned does that.
func Parse(raw string) (Ref, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Ref{}, fmt.Errorf("%q is not an absolute URL", raw)
	}
	project, rest, ok := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/-/blob/")
	if !ok || project == "" {
		return Ref{}, fmt.Errorf("%q is not a /-/blob/ file URL", raw)
	}
	tag, path, ok := strings.Cut(rest, "/")
	if !ok || tag == "" || path == "" {
		return Ref{}, fmt.Errorf("%q names no ref and path", raw)
	}
	return Ref{Raw: raw, Host: u.Host, Project: project, Tag: tag, Path: path, Anchor: u.Fragment}, nil
}

// Pinned reports why a reference is not pinned, or nil when its form is
// pinned: a ref that is not a branch name or a bare commit, and an anchor.
// Whether the tag exists is Resolve's question, not this one.
func (r Ref) Pinned() error {
	switch {
	case branchNames[r.Tag]:
		return fmt.Errorf("%s is pinned to branch %q, not a release tag", r.Raw, r.Tag)
	case commitSHA.MatchString(r.Tag):
		return fmt.Errorf("%s is pinned to commit %q, not a release tag", r.Raw, r.Tag)
	case r.Anchor == "":
		return fmt.Errorf("%s has no requirement or interface anchor", r.Raw)
	}
	return nil
}

// CheckPinnedDocument is CheckPinned for a reference to a whole document,
// such as a story's parent epic: the ref must be pinned, and an anchor is
// optional.
func CheckPinnedDocument(raw string) error {
	r, err := Parse(raw)
	if err != nil {
		return err
	}
	if r.Anchor == "" {
		r.Anchor = "document"
	}
	return r.Pinned()
}

// CheckPinned parses raw and reports any malformed or unpinned form.
func CheckPinned(raw string) error {
	r, err := Parse(raw)
	if err != nil {
		return err
	}
	return r.Pinned()
}
