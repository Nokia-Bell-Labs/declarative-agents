// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// IssuesDir holds one file per issue, <id>.json, in a tracker whose config
// sets layout: files. A write rewrites only the issues it changes, so its
// cost does not grow with the tracker, and writers of different issues
// change different files, which git merges by itself (eng04).
const IssuesDir = ".beads/issues"

// IndexDir maps each repository's IIDs to issue IDs in the files layout:
// .beads/index/<repository>/<iid> holds the ID, with the repository's path
// escaped into one directory name. A lookup by IID reads one entry and a
// create numbers its issue from the entries' names, so neither reads the
// issues. Entries only appear, as an IID is never reused, so git merges two
// clones' entries by themselves, except where both created the same IID.
const IndexDir = ".beads/index"

// LayoutFiles is the config's layout value for one file per issue. Without
// it the tracker keeps every issue in IssuesFile, bd's JSONL.
const LayoutFiles = "files"

// files reports whether the clone's tracker keeps one file per issue.
func (s *Store) files() bool {
	c, err := s.config()
	return err == nil && c.Layout == LayoutFiles
}

func issuePath(dir, id string) string { return filepath.Join(dir, IssuesDir, id+".json") }

// encodeIssue is one issue as its file holds it: one line of JSON, so that
// two writers changing the same issue always conflict in git and the merge
// goes through mergeIssue, never through git's line merge.
func encodeIssue(i Issue) ([]byte, error) {
	if i.Type == "" {
		i.Type = "issue"
	}
	b, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func decodeIssue(path string, b []byte) (Issue, error) {
	var i Issue
	if err := json.Unmarshal(bytes.TrimSpace(b), &i); err != nil {
		return i, fmt.Errorf("%s: %w", path, err)
	}
	return i, nil
}

// readIssueDir reads every issue file under dir, in ID order.
func readIssueDir(dir string) ([]Issue, error) {
	entries, err := os.ReadDir(filepath.Join(dir, IssuesDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var issues []Issue
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, IssuesDir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		i, err := decodeIssue(path, b)
		if err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, nil
}

// IndexEntry is the path, within a tracker checkout, of the index entry for
// iid in repository.
func IndexEntry(repository string, iid int) string {
	return filepath.Join(IndexDir, url.PathEscape(repository), strconv.Itoa(iid))
}

func indexPath(dir, repository string, iid int) string {
	return filepath.Join(dir, IndexEntry(repository, iid))
}

// topIID is the highest IID in repository, read from the names of its
// index entries.
func topIID(dir, repository string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(dir, IndexDir, url.PathEscape(repository)))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	top := 0
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil {
			top = max(top, n)
		}
	}
	return top, nil
}

// indexID is the ID the index names for iid in repository, or "" when it
// names none.
func indexID(dir, repository string, iid int) string {
	b, err := os.ReadFile(indexPath(dir, repository, iid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// writeIndex records i's entry unless it already names i.
func writeIndex(dir string, i Issue) error {
	if i.IID == 0 || i.Repository == "" {
		return nil
	}
	if indexID(dir, i.Repository, i.IID) == i.ID {
		return nil
	}
	p := indexPath(dir, i.Repository, i.IID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(i.ID+"\n"), 0o644)
}

// ensureIndex builds the index of a tracker that keeps issues as files but
// has none, one written before the index existed, and reports how many
// entries it wrote. A tracker with an index directory has a whole one: every
// write since keeps it.
func ensureIndex(dir string) (int, error) {
	if _, err := os.Stat(filepath.Join(dir, IndexDir)); err == nil {
		return 0, nil
	}
	issues, err := readIssueDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, i := range issues {
		if i.IID == 0 || i.Repository == "" {
			continue
		}
		if err := writeIndex(dir, i); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// encodedAll is each issue's file content by ID, what a write compares
// against to rewrite only the issues it changed.
func encodedAll(issues []Issue) (map[string][]byte, error) {
	out := make(map[string][]byte, len(issues))
	for _, i := range issues {
		b, err := encodeIssue(i)
		if err != nil {
			return nil, err
		}
		out[i.ID] = b
	}
	return out, nil
}

// writeChanged writes the files of the issues whose content differs from
// before, with their index entries, and reports whether it wrote any. A
// tracker without an index gets one first, so no write leaves a partial one.
func writeChanged(dir string, before map[string][]byte, issues []Issue) (bool, error) {
	if err := os.MkdirAll(filepath.Join(dir, IssuesDir), 0o755); err != nil {
		return false, err
	}
	if _, err := ensureIndex(dir); err != nil {
		return false, err
	}
	wrote := false
	for _, i := range issues {
		b, err := encodeIssue(i)
		if err != nil {
			return false, err
		}
		if bytes.Equal(before[i.ID], b) {
			continue
		}
		if err := os.WriteFile(issuePath(dir, i.ID), b, 0o644); err != nil {
			return false, err
		}
		if err := writeIndex(dir, i); err != nil {
			return false, err
		}
		wrote = true
	}
	return wrote, nil
}

// mergeFiles records a merge with the tracker repository in the files
// layout. git merges the issues only one side changed; an issue both sides
// changed conflicts, as its file is one line, and merges by mergeIssue; an
// issue each side created with the same IID in the same repository is
// renumbered on ours, as mergeIssues does, its index entry keeps theirs and
// ours gets a new one; a record file both sides changed merges by its own
// rules (eng04). It runs while a rejected writer retries, and every other
// writer's push in that time rejects it again, so it does only what both
// sides' changes call for.
func (s *Store) mergeFiles(base, remote string) error {
	ours, err := s.changed(base, "HEAD")
	if err != nil {
		return err
	}
	theirs, err := s.changed(base, remote)
	if err != nil {
		return err
	}
	conflicted := ""
	if _, err := s.git("merge", "-q", "--no-ff", "--no-commit", remote); err != nil {
		if _, herr := s.git("rev-parse", "-q", "--verify", "MERGE_HEAD"); herr != nil {
			return err
		}
		if conflicted, err = s.git("diff", "--name-only", "--diff-filter=U"); err != nil {
			return err
		}
	}
	for _, p := range strings.Fields(conflicted) {
		if strings.HasPrefix(p, IndexDir+"/") {
			// Both sides created this IID: theirs keeps it, and
			// renumberNew gives ours the next one.
			if _, err := s.git("checkout", "--theirs", "--", p); err != nil {
				return err
			}
			continue
		}
		if !strings.HasPrefix(p, IssuesDir+"/") {
			continue // a record file, merged below
		}
		var sides [3]Issue
		for n := range sides {
			out, err := s.git("show", fmt.Sprintf(":%d:%s", n+1, p))
			if err != nil {
				continue // the side has no version: an issue new on the others
			}
			if sides[n], err = decodeIssue(p, []byte(out)); err != nil {
				return err
			}
		}
		rows, err := s.states()
		if err != nil {
			return err
		}
		b, err := encodeIssue(mergeIssue(rows, sides[0], sides[1], sides[2], s.stamp()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(s.Dir, p), b, 0o644); err != nil {
			return err
		}
	}
	if err := s.renumberNew(added(ours), added(theirs)); err != nil {
		return err
	}
	if ours[MilestonesFile] != "" && theirs[MilestonesFile] != "" {
		if err := mergeRecordFile[Milestone](s, MilestonesFile, base, remote); err != nil {
			return err
		}
	}
	if ours[LabelsFile] != "" && theirs[LabelsFile] != "" {
		if err := mergeRecordFile[Label](s, LabelsFile, base, remote); err != nil {
			return err
		}
	}
	if _, err := s.git("add", "-A", "--", ".beads"); err != nil {
		return err
	}
	_, err = s.git("commit", "-q", "-m", "scrum-master: merge the tracker repository")
	return err
}

// changed is the tracker files rev changed since base, each with its git
// status letter.
func (s *Store) changed(base, rev string) (map[string]string, error) {
	out, err := s.git("diff", "--name-status", "--no-renames", base, rev, "--", ".beads")
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if status, path, ok := strings.Cut(line, "\t"); ok {
			files[path] = status
		}
	}
	return files, nil
}

// added is the issue files among changed that are new, in path order.
func added(changed map[string]string) []string {
	var paths []string
	for p, status := range changed {
		if status == "A" && strings.HasPrefix(p, IssuesDir+"/") {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return paths
}

// renumberNew gives each issue ours created since base that collides with
// one theirs created the next free IID in its repository. Only issues
// created on both sides can collide: an existing issue's IID is at most
// base's highest, and a new one is above it.
func (s *Store) renumberNew(ours, theirs []string) error {
	if len(ours) == 0 || len(theirs) == 0 {
		return nil
	}
	var issues []Issue
	for _, p := range append(slices.Clone(theirs), ours...) {
		b, err := os.ReadFile(filepath.Join(s.Dir, p))
		if err != nil {
			return err
		}
		i, err := decodeIssue(p, b)
		if err != nil {
			return err
		}
		issues = append(issues, i)
	}
	before, err := encodedAll(issues)
	if err != nil {
		return err
	}
	oursNew := map[string]bool{}
	for _, p := range ours {
		oursNew[strings.TrimSuffix(filepath.Base(p), ".json")] = true
	}
	renumber(issues, func(id string) bool { return oursNew[id] })
	_, err = writeChanged(s.Dir, before, issues)
	return err
}

// MigrateResult is what scrum-master tracker migrate did.
type MigrateResult struct {
	Issues  int    `json:"issues"`
	Commit  string `json:"tracker_commit,omitempty"`
	Already bool   `json:"already"`
	// Indexed counts the index entries built for a tracker that already
	// kept one file per issue but had no index.
	Indexed int `json:"indexed"`
}

// Migrate moves a tracker from IssuesFile to one file per issue, with its
// index, in one commit that also sets the config's layout; a tracker that
// already keeps one file per issue but has no index gets one. It is meant to
// run once, when no one is writing: a tracker that moved while it ran is
// left as it was, and the command asks to be run again.
func (s *Store) Migrate() (MigrateResult, error) {
	var r MigrateResult
	fr, err := s.Sync()
	if err != nil {
		return r, err
	}
	if fr.Offline {
		return r, errors.New("the tracker repository is unreachable; migrate needs it")
	}
	before, err := s.git("rev-parse", "HEAD")
	if err != nil {
		return r, err
	}
	if s.files() {
		r.Already = true
		if r.Indexed, err = ensureIndex(s.Dir); err != nil || r.Indexed == 0 {
			return r, err
		}
		return r, s.commitMigration(&r, before, fmt.Sprintf("scrum-master: index the tracker's %d issues (%s)", r.Indexed, s.User))
	}
	issues, err := s.read()
	if err != nil {
		return r, err
	}
	if _, err := writeChanged(s.Dir, nil, issues); err != nil {
		return r, err
	}
	if err := os.Remove(filepath.Join(s.Dir, IssuesFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return r, err
	}
	c, err := s.config()
	if err != nil {
		return r, err
	}
	c.Layout = LayoutFiles
	if err := WriteConfig(s.Dir, c); err != nil {
		return r, err
	}
	r.Issues = len(issues)
	return r, s.commitMigration(&r, before, fmt.Sprintf("scrum-master: migrate the tracker to one file per issue, %d issues (%s)", len(issues), s.User))
}

// commitMigration commits and pushes what Migrate wrote, once: a rejected
// push resets the clone to before and asks for another run.
func (s *Store) commitMigration(r *MigrateResult, before, message string) error {
	if _, err := s.git("add", "-A", "--", ".beads"); err != nil {
		return err
	}
	if _, err := s.git("commit", "-q", "-m", message); err != nil {
		return err
	}
	if _, err := s.git("push", "-q", "origin", "HEAD:"+s.branch()); err != nil {
		if _, rerr := s.git("reset", "-q", "--hard", before); rerr != nil {
			return errors.Join(err, rerr)
		}
		return fmt.Errorf("the tracker repository moved while migrating, so nothing was written; run scrum-master tracker migrate again when no one is writing: %w", err)
	}
	var err error
	r.Commit, err = s.git("rev-parse", "HEAD")
	return err
}

// Export writes every issue as bd's JSONL, one line per issue sorted by
// ID, whichever layout the tracker keeps, so bd can import a tracker that
// keeps one file per issue.
func (s *Store) Export() ([]byte, error) {
	if _, err := s.Sync(); err != nil {
		return nil, err
	}
	issues, err := s.read()
	if err != nil {
		return nil, err
	}
	return encodeJSONL(issues)
}
