// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package checkout is the code half every tracking system shares: a
// project's files, tags, branches, and merged history, read from its local
// checkout and that checkout's origin (srd042 R2.3). bd, gh, and glab differ
// in their issues and in how a branch is reviewed, never in this.
package checkout

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Code answers the code half for the command's checkout at Dir. A
// project's checkout is the one Locate finds: Dir's own when its origin is
// the project, else the nearest checkout named for the project, beside Dir
// or beside a folder above it, whose origin is the project. A checkout
// whose origin names no forge project, a local path or none, answers for
// any project it finds no checkout of, as before; one whose origin is
// another project never does (GH-2744).
type Code struct {
	Dir string
	// fetched records the projects whose origin this command has fetched,
	// so files read reflect what other clones pushed, at one fetch each.
	fetched map[string]bool
}

func (c *Code) Checkout(project string) string {
	if project == "" {
		return c.Dir
	}
	if dir, ok := Locate(c.Dir, project); ok {
		return dir
	}
	if origin := Origin(toplevel(c.Dir)); origin == "" || strings.HasPrefix(origin, "/") || strings.HasPrefix(origin, ".") {
		return c.Dir
	}
	return ""
}

// HasCheckout reports whether project has a checkout of its own to read,
// as Locate finds one; the fallback Checkout keeps for a checkout with no
// forge origin is not one (GH-2725).
func (c *Code) HasCheckout(project string) bool {
	if project == "" {
		return false
	}
	_, ok := Locate(c.Dir, project)
	return ok
}

// noCheckout is the error for a project Checkout finds no checkout of,
// naming the folders searched.
func (c *Code) noCheckout(project string) error {
	return fmt.Errorf("%s: %w in %s; check it out beside this one, or in a folder its group mirrors", project, ErrNoCheckout, strings.Join(Levels(c.Dir), ", "))
}

func (c *Code) Git(project string, args ...string) (string, error) {
	out, err := c.raw(project, args...)
	return strings.TrimSpace(string(out)), err
}

func (c *Code) raw(project string, args ...string) ([]byte, error) {
	dir := c.Checkout(project)
	if dir == "" {
		return nil, c.noCheckout(project)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func (c *Code) DefaultBranch(project string) (string, error) {
	ref, err := c.Git(project, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "main", nil
	}
	return strings.TrimPrefix(ref, "origin/"), nil
}

func (c *Code) BranchExists(project, branch string) (bool, error) {
	out, err := c.Git(project, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	return out != "", err
}

func (c *Code) CreateBranch(project, branch, ref string) error {
	if _, err := c.Git(project, "fetch", "-q", "origin", ref); err != nil {
		return err
	}
	_, err := c.Git(project, "push", "-q", "origin", "refs/remotes/origin/"+ref+":refs/heads/"+branch)
	return err
}

func (c *Code) rev(project, ref string) string {
	if !c.fetched[project] {
		if c.fetched == nil {
			c.fetched = map[string]bool{}
		}
		c.fetched[project] = true
		c.Git(project, "fetch", "-q", "origin")
	}
	if _, err := c.Git(project, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+ref); err == nil {
		return "origin/" + ref
	}
	return ref
}

// OpenBranch opens branch from ref with one empty commit of its own,
// message, for a host that opens no review of a branch with nothing over
// its base.
func (c *Code) OpenBranch(project, branch, ref, message string) error {
	if _, err := c.Git(project, "fetch", "-q", "origin", ref); err != nil {
		return err
	}
	tree, err := c.Git(project, "rev-parse", "refs/remotes/origin/"+ref+"^{tree}")
	if err != nil {
		return err
	}
	commit, err := c.Git(project, "commit-tree", tree, "-p", "refs/remotes/origin/"+ref, "-m", message)
	if err != nil {
		return err
	}
	_, err = c.Git(project, "push", "-q", "origin", commit+":refs/heads/"+branch)
	return err
}

// GetFile reads p at ref as text: through the repository's textconv, so a
// git-crypt repository's files come back decrypted, and as stored where no
// textconv is configured.
func (c *Code) GetFile(project, p, ref string) ([]byte, error) {
	out, err := c.raw(project, "cat-file", "--textconv", c.rev(project, ref)+":"+p)
	if errors.Is(err, ErrNoCheckout) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("%s at %s: %w", p, ref, tracker.ErrNotFound)
	}
	return out, nil
}

func (c *Code) ListDir(project, dir, ref string) ([]string, error) {
	out, err := c.Git(project, "ls-tree", "--name-only", c.rev(project, ref), strings.TrimSuffix(dir, "/")+"/")
	if errors.Is(err, ErrNoCheckout) {
		return nil, err
	}
	if err != nil || out == "" {
		return nil, fmt.Errorf("%s at %s: %w", dir, ref, tracker.ErrNotFound)
	}
	return strings.Split(out, "\n"), nil
}

// LastCommit is the commit that last changed p at ref.
func (c *Code) LastCommit(project, p, ref string) (string, error) {
	out, err := c.Git(project, "log", "-1", "--format=%H", c.rev(project, ref), "--", p)
	if errors.Is(err, ErrNoCheckout) {
		return "", err
	}
	if err != nil || out == "" {
		return "", fmt.Errorf("%s at %s: %w", p, ref, tracker.ErrNotFound)
	}
	return out, nil
}

func (c *Code) TagExists(project, tag string) (bool, error) {
	out, err := c.Git(project, "ls-remote", "--tags", "origin", "refs/tags/"+tag)
	return out != "", err
}

// mergedBy is the Merged-by trailer scrum-master issue merge writes, naming the
// tracker user who merged.
var mergedBy = regexp.MustCompile(`(?m)^Merged-by: (\S+)$`)

// citation is a project#iid reference in a merge commit's message.
var citation = regexp.MustCompile(`[A-Za-z0-9_.\-]+(?:/[A-Za-z0-9_.\-]+)*#\d+`)

// Merges reads a project's merged history from the merge
// commits on its default branch, which are its deliveries (srd042 R2.3). A merge is numbered by its place in the branch's
// first-parent history, so the number is stable, and named by its commit, as it has no merge
// request; its author is the tracker
// user its Merged-by trailer names, which scrum-master issue merge writes, else the
// merge commit's git author; its merge date is the commit's UTC day, and
// since is compared with the commit's instant, never a local date; and it closes every
// issue its message cites, written into the description as
// Closes lines. The branch read is whichever of the local and origin
// default branches is the newer.
func (c *Code) Merges(project, author, since string) ([]tracker.Merge, error) {
	from, err := time.Parse(time.RFC3339, since)
	if err != nil {
		if from, err = time.Parse("2006-01-02", since); err != nil {
			return nil, fmt.Errorf("since %q is neither YYYY-MM-DD nor an RFC 3339 instant", since)
		}
	}
	base, err := c.DefaultBranch(project)
	if err != nil {
		return nil, err
	}
	c.Git(project, "fetch", "-q", "origin", base)
	ref := "refs/heads/" + base
	if _, err := c.Git(project, "rev-parse", "--verify", "-q", ref); err != nil {
		ref = "refs/remotes/origin/" + base
	} else if _, err := c.Git(project, "merge-base", "--is-ancestor", "refs/remotes/origin/"+base, ref); err != nil {
		if _, err := c.Git(project, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+base); err == nil {
			ref = "refs/remotes/origin/" + base
		}
	}
	log, err := c.Git(project, "log", "--first-parent", "--merges", "--reverse", "--format=%h%x1f%an%x1f%cI%x1f%B%x1e", ref)
	if err != nil {
		return nil, err
	}
	var out []tracker.Merge
	n := 0
	for _, entry := range strings.Split(log, "\x1e") {
		fields := strings.SplitN(strings.TrimLeft(entry, "\n"), "\x1f", 4)
		if len(fields) != 4 {
			continue
		}
		n++
		at, err := time.Parse(time.RFC3339, fields[2])
		if err != nil {
			return nil, fmt.Errorf("merge commit date %q: %w", fields[2], err)
		}
		commit, by, message := fields[0], fields[1], strings.TrimSpace(fields[3])
		if m := mergedBy.FindStringSubmatch(message); m != nil {
			by = m[1]
			message = strings.TrimSpace(mergedBy.ReplaceAllString(message, ""))
		}
		if at.Before(from) || (author != "" && by != author) {
			continue
		}
		title, body, _ := strings.Cut(message, "\n")
		description := strings.TrimSpace(body)
		for _, ref := range citation.FindAllString(message, -1) {
			description += "\n\nCloses " + ref
		}
		out = append(out, tracker.Merge{IID: n, Title: title, Description: strings.TrimSpace(description),
			TargetBranch: base, State: "merged", Author: by, MergedAt: at.UTC().Format("2006-01-02"), Commit: commit})
	}
	return out, nil
}
