// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// FindDocument is the repository path of a signed document (program.SignedDocument)
// in the checkout root: named by its path, or by its id, read from its frontmatter
// for the documents whose file is not named for it.
func FindDocument(root, name string) (string, error) {
	exists := func(p string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		return err == nil
	}
	if strings.Contains(name, "/") {
		if !program.SignedDocument(name) {
			return "", fmt.Errorf("%s carries no reviewer: it is not the vision, the architecture, the road map, an SRD, a use case, or a planning document", name)
		}
		if !exists(name) {
			return "", fmt.Errorf("%s holds no %s", root, name)
		}
		return name, nil
	}
	for _, p := range []string{hierarchy.PathOf(name), path.Join(program.SRDDir, name+".md"), path.Join("docs/specs/use-cases", name+".md"), path.Join(hierarchy.ObjectiveDir, name+".md")} {
		if p != "" && exists(p) {
			return p, nil
		}
	}
	for _, p := range []string{"docs/VISION.md", hierarchy.ArchitectureFile, "docs/road-map.md"} {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		if front, err := corpus.Front(b); err == nil && front["id"] == name {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s holds no document %s", root, name)
}

// ReviewerResult is what scrum-master backlog reviewer did.
type ReviewerResult struct {
	Path     string `json:"path"`
	Reviewer string `json:"reviewer,omitempty"`
	Previous string `json:"previous,omitempty"`
	Action   string `json:"action"`
	Commit   string `json:"commit,omitempty"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

// SetReviewer writes the document's reviewer into its frontmatter, or, with an
// empty person, drops it (srd013 R11.4), replacing another only with force. A
// change to it alone needs no sign-off, so a reviewer who is away can be replaced.
func SetReviewer(dir, name, person string, force, commit, dryRun bool) (ReviewerResult, error) {
	r := ReviewerResult{Reviewer: person, Action: "unchanged", DryRun: dryRun}
	root, err := toplevelOf(dir)
	if err != nil {
		return r, err
	}
	if r.Path, err = FindDocument(root, name); err != nil {
		return r, err
	}
	file := filepath.Join(root, filepath.FromSlash(r.Path))
	doc, err := os.ReadFile(file)
	if err != nil {
		return r, err
	}
	front, err := corpus.Front(doc)
	if err != nil {
		return r, fmt.Errorf("%s: %w", r.Path, err)
	}
	r.Previous, _ = front[corpus.FrontReviewer].(string)
	if r.Previous != "" && r.Previous != person && person != "" && !force {
		return r, fmt.Errorf("%s names %s as its reviewer; --force replaces them", r.Path, r.Previous)
	}
	next, err := corpus.SetFront(doc, corpus.FrontReviewer, person)
	if err != nil {
		return r, fmt.Errorf("%s: %w", r.Path, err)
	}
	if bytes.Equal(next, doc) {
		return r, nil
	}
	r.Action = "named"
	if person == "" {
		r.Action = "dropped"
	}
	if dryRun {
		return r, nil
	}
	if err := os.WriteFile(file, next, 0o644); err != nil {
		return r, err
	}
	if !commit {
		return r, nil
	}
	if out, err := exec.Command("git", "-C", root, "add", "--", filepath.FromSlash(r.Path)).CombinedOutput(); err != nil {
		return r, fmt.Errorf("add %s: %v: %s", r.Path, err, bytes.TrimSpace(out))
	}
	msg := fmt.Sprintf("%s: reviewer %s (scrum-master backlog reviewer)", r.Path, r.Action)
	if out, err := exec.Command("git", "-C", root, "commit", "-q", "-m", msg).CombinedOutput(); err != nil {
		return r, fmt.Errorf("commit %s: %v: %s", r.Path, err, bytes.TrimSpace(out))
	}
	sha, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return r, err
	}
	r.Commit = strings.TrimSpace(string(sha))
	return r, nil
}

// AuthorsResult is what scrum-master backlog author did.
type AuthorsResult struct {
	Path    string   `json:"path"`
	Authors []string `json:"authors,omitempty"`
	Action  string   `json:"action"`
	Commit  string   `json:"commit,omitempty"`
	DryRun  bool     `json:"dry_run,omitempty"`
}

// SetAuthor adds person to the document's authors, or takes them off, in its
// frontmatter (srd045 R2.6), keeping those already named in their order. A change
// to it alone needs no sign-off, as it says who rather than what.
func SetAuthor(dir, name, person string, remove, commit, dryRun bool) (AuthorsResult, error) {
	r := AuthorsResult{Action: "unchanged", DryRun: dryRun}
	root, err := toplevelOf(dir)
	if err != nil {
		return r, err
	}
	if r.Path, err = FindDocument(root, name); err != nil {
		return r, err
	}
	file := filepath.Join(root, filepath.FromSlash(r.Path))
	doc, err := os.ReadFile(file)
	if err != nil {
		return r, err
	}
	front, err := corpus.Front(doc)
	if err != nil {
		return r, fmt.Errorf("%s: %w", r.Path, err)
	}
	authors := slices.DeleteFunc(corpus.FrontNames(front, corpus.FrontAuthors), func(n string) bool { return n == AuthorPlaceholder })
	r.Authors = authors
	has := slices.Contains(authors, person)
	if has != remove {
		return r, nil
	}
	if remove {
		r.Action = "removed"
		authors = slices.DeleteFunc(authors, func(n string) bool { return n == person })
	} else {
		r.Action = "added"
		authors = append(authors, person)
	}
	r.Authors = authors
	if dryRun {
		return r, nil
	}
	next, err := corpus.SetFront(doc, corpus.FrontAuthors, strings.Join(authors, ", "))
	if err != nil {
		return r, fmt.Errorf("%s: %w", r.Path, err)
	}
	if err := os.WriteFile(file, next, 0o644); err != nil {
		return r, err
	}
	if !commit {
		return r, nil
	}
	if out, err := exec.Command("git", "-C", root, "add", "--", filepath.FromSlash(r.Path)).CombinedOutput(); err != nil {
		return r, fmt.Errorf("add %s: %v: %s", r.Path, err, bytes.TrimSpace(out))
	}
	msg := fmt.Sprintf("%s: author %s (scrum-master backlog author)", r.Path, r.Action)
	if out, err := exec.Command("git", "-C", root, "commit", "-q", "-m", msg).CombinedOutput(); err != nil {
		return r, fmt.Errorf("commit %s: %v: %s", r.Path, err, bytes.TrimSpace(out))
	}
	sha, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return r, err
	}
	r.Commit = strings.TrimSpace(string(sha))
	return r, nil
}

// signedDocuments are the signed documents git tracks in the checkout at dir.
func signedDocuments(dir string) []string {
	out, err := exec.Command("git", "-C", dir, "ls-files", "docs").Output()
	if err != nil {
		return nil
	}
	var docs []string
	for _, p := range strings.Fields(string(out)) {
		if program.SignedDocument(p) {
			docs = append(docs, p)
		}
	}
	return docs
}

// authorChecks holds documents to naming their authors (srd045 R2.6): a document
// the latest release's tag does not hold is new and must, and one it holds is
// reported, once for them all, so a program does not fail on the day this lands. A
// program with no release has no line to draw, so its documents are reported the
// same way.
func authorChecks(dir, tag string) (findings []hierarchy.Finding, warning *hierarchy.Finding) {
	var older []string
	for _, p := range signedDocuments(dir) {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		front, err := corpus.Front(b)
		if err != nil {
			continue
		}
		named := slices.DeleteFunc(corpus.FrontNames(front, corpus.FrontAuthors), func(n string) bool { return n == AuthorPlaceholder })
		if len(named) > 0 {
			continue
		}
		if tag != "" && exec.Command("git", "-C", dir, "cat-file", "-e", tag+":./"+p).Run() != nil {
			findings = append(findings, hierarchy.Finding{File: p, Field: corpus.FrontAuthors, Message: "a new document names its authors, as tracker usernames; scrum-master backlog author adds one"})
			continue
		}
		older = append(older, p)
	}
	if len(older) > 0 {
		sample := older[:min(3, len(older))]
		warning = &hierarchy.Finding{File: "docs", Field: corpus.FrontAuthors, Message: fmt.Sprintf("%d documents name no authors, such as %s; scrum-master backlog author adds one", len(older), strings.Join(sample, ", "))}
	}
	return findings, warning
}

func init() {
	var au struct {
		dir, document, person string
		remove, commit        bool
	}
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "author", Summary: "add or remove a document's author, in its frontmatter; agents never run it",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&au.dir, "dir", ".", "a checkout of the repository holding the document")
			fs.StringVar(&au.document, "document", "", "the document, by id or path (required)")
			fs.StringVar(&au.person, "person", "", "the author's tracker username (required)")
			fs.BoolVar(&au.remove, "remove", false, "take them off instead")
			fs.BoolVar(&au.commit, "commit", false, "commit the changed document on the current branch")
		},
		Run: func(ctx *cli.Context) error {
			if au.document == "" || au.person == "" {
				return fmt.Errorf("%w: author needs --document and --person", cli.ErrUsage)
			}
			if strings.ContainsAny(au.person, " \t/\\,") {
				return fmt.Errorf("%w: %q is not a username", cli.ErrUsage, au.person)
			}
			r, err := SetAuthor(au.dir, au.document, au.person, au.remove, au.commit, ctx.DryRun)
			if err != nil {
				return err
			}
			action := r.Action
			if r.DryRun && action != "unchanged" {
				action = "would be " + action
			}
			h := fmt.Sprintf("%s author %s: %s\n", r.Path, au.person, action)
			if r.Commit != "" {
				h += "committed " + r.Commit + "\n"
			}
			return ctx.Emit(r, h)
		},
	})

	var a struct {
		dir, document, person string
		force, clear, commit  bool
	}
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "reviewer", Summary: "name who signs off on a document, in its frontmatter; agents never run it",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&a.dir, "dir", ".", "a checkout of the repository holding the document")
			fs.StringVar(&a.document, "document", "", "the document, by id or path (required)")
			fs.StringVar(&a.person, "person", "", "the tracker username who signs off")
			fs.BoolVar(&a.clear, "clear", false, "drop the reviewer instead of naming someone")
			fs.BoolVar(&a.force, "force", false, "replace the reviewer the document already names")
			fs.BoolVar(&a.commit, "commit", false, "commit the changed document on the current branch")
		},
		Run: func(ctx *cli.Context) error {
			if a.document == "" || (a.person == "") == !a.clear {
				return fmt.Errorf("%w: reviewer needs --document and one of --person or --clear", cli.ErrUsage)
			}
			if strings.ContainsAny(a.person, " \t/\\") {
				return fmt.Errorf("%w: %q is not a username", cli.ErrUsage, a.person)
			}
			r, err := SetReviewer(a.dir, a.document, a.person, a.force, a.commit, ctx.DryRun)
			if err != nil {
				return err
			}
			action := r.Action
			if r.DryRun && action != "unchanged" {
				action = "would be " + action
			}
			h := fmt.Sprintf("%s reviewer: %s\n", r.Path, action)
			if r.Commit != "" {
				h += "committed " + r.Commit + "\n"
			}
			return ctx.Emit(r, h)
		},
	})
}
