// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package repofiles reads files from the program's other repositories on
// their default branch: through the host's API when the tracking system
// serves files without a checkout, so a reader of many repositories clones
// none of them, else from a checkout of the repository beside this one,
// never from another repository's checkout taken for it. Each file comes
// with the commit that last changed it, and a repository that cannot be
// read says why (GH-2725, GH-2726).
package repofiles

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Where a file was read.
const (
	SourceHost     = "host"
	SourceCheckout = "checkout"
)

// File is one file read from a repository.
type File struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Commit     string `json:"commit"`
	Source     string `json:"source"`
	Content    string `json:"content"`
}

// Unread is a repository a reader could not read, and why.
type Unread struct {
	Repository string `json:"repository"`
	Reason     string `json:"reason"`
}

// Read reads every file under dir in repo that keep accepts, each with the
// commit that last changed it: from the host when it serves files, else
// from a checkout of repo. A repository it cannot read gives the reason; a
// dir that holds nothing gives neither files nor a reason.
func Read(t tracker.Tracker, repo, dir string, keep func(path string) bool) ([]File, string) {
	sys := tracker.For(t, repo)
	if host, ok := sys.(tracker.HostFiles); ok {
		paths, err := host.HostTree(repo, dir)
		if err == nil || errors.Is(err, tracker.ErrNotFound) {
			var out []File
			for _, p := range paths {
				if !keep(p) {
					continue
				}
				content, commit, err := host.HostFile(repo, p)
				if err != nil {
					return out, fmt.Sprintf("%s: %v", p, err)
				}
				out = append(out, File{Repository: repo, Path: p, Commit: commit, Source: SourceHost, Content: string(content)})
			}
			return out, ""
		}
	}
	if co, ok := sys.(tracker.Checkouts); ok && !co.HasCheckout(repo) {
		return nil, "the host serves no files and no checkout of it sits beside this one; scrum-master checkouts lays it down (GH-2812)"
	}
	ref, err := sys.DefaultBranch(repo)
	if err != nil {
		return nil, err.Error()
	}
	paths, err := Tree(sys, repo, dir, ref)
	if err != nil {
		return nil, err.Error()
	}
	var out []File
	for _, p := range paths {
		if !keep(p) {
			continue
		}
		content, err := sys.GetFile(repo, p, ref)
		if err != nil {
			return out, fmt.Sprintf("%s: %v", p, err)
		}
		commit := ""
		if fc, ok := sys.(tracker.FileCommits); ok {
			if commit, err = fc.LastCommit(repo, p, ref); err != nil {
				return out, fmt.Sprintf("%s: %v", p, err)
			}
		}
		out = append(out, File{Repository: repo, Path: p, Commit: commit, Source: SourceCheckout, Content: string(content)})
	}
	return out, ""
}

// Tree lists the files under dir at any depth through the code half; a
// dir that is not there holds nothing. An entry with an extension is a
// file; any other is a directory, listed in turn.
func Tree(f tracker.Forge, repo, dir, ref string) ([]string, error) {
	entries, err := f.ListDir(repo, dir, ref)
	if errors.Is(err, tracker.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !strings.Contains(e, "/") {
			e = path.Join(dir, e)
		}
		if path.Ext(e) != "" {
			out = append(out, e)
			continue
		}
		sub, err := Tree(f, repo, e, ref)
		if err != nil {
			return out, err
		}
		out = append(out, sub...)
	}
	return out, nil
}

// Materialize writes files into a new directory under parent, at their
// paths, so a reader of a checkout reads them as one; the caller removes
// it.
func Materialize(parent string, files []File) (string, error) {
	dir, err := os.MkdirTemp(parent, "repo-")
	if err != nil {
		return "", err
	}
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return dir, err
		}
		if err := os.WriteFile(p, []byte(f.Content), 0o644); err != nil {
			return dir, err
		}
	}
	return dir, nil
}
