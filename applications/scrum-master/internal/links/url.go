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
// https://HOST/GROUP/REPO/-/blob/TAG/PATH#ANCHOR, or on GitHub
// https://HOST/OWNER/REPO/blob/TAG/PATH#ANCHOR (srd007 R5.1).
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
	project, rest, ok := splitBlob(strings.TrimPrefix(u.Path, "/"))
	if !ok || project == "" {
		return Ref{}, fmt.Errorf("%q is not a /-/blob/ file URL, nor OWNER/REPO/blob/ on GitHub", raw)
	}
	tag, path, ok := splitRef(rest)
	if !ok || tag == "" || path == "" {
		return Ref{}, fmt.Errorf("%q names no ref and path", raw)
	}
	return Ref{Raw: raw, Host: u.Host, Project: project, Tag: tag, Path: path, Anchor: u.Fragment}, nil
}

// splitBlob cuts a URL's path at its file marker into the project and what
// follows: GitLab's /-/blob/, under groups of any depth, else GitHub's blob as
// the segment after OWNER/REPO, which has no groups to mistake it for (GH-2901).
func splitBlob(p string) (project, rest string, ok bool) {
	if project, rest, ok = strings.Cut(p, "/-/blob/"); ok {
		return project, rest, true
	}
	seg := strings.SplitN(p, "/", 4)
	if len(seg) == 4 && seg[2] == "blob" {
		return seg[0] + "/" + seg[1], seg[3], true
	}
	return "", "", false
}

// splitRef cuts what follows the file marker into the ref and the path. A ref
// may hold a slash, as the corpus's own tags do (corpus/v0.1.0), and a URL does
// not say where it ends, so a corpus document's docs/ folder marks it, else the
// first slash (GH-2901).
func splitRef(rest string) (ref, path string, ok bool) {
	if i := strings.Index(rest, "/docs/"); i > 0 {
		return rest[:i], rest[i+1:], true
	}
	return strings.Cut(rest, "/")
}

// BlobURL is the file's page at ref under webBase, in the form the host serves
// (srd007 R5.1, GH-2901).
func BlobURL(webBase, project, ref, path string) string {
	marker := "/-/blob/"
	if strings.Contains(webBase, "github") {
		marker = "/blob/"
	}
	return strings.TrimSuffix(webBase, "/") + "/" + project + marker + ref + "/" + path
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
