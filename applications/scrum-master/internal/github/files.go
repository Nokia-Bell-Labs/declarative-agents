// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package github

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

var _ tracker.HostFiles = (*Tracker)(nil)

// HostTree lists the files under dir on the repository's default branch
// from GitHub's git trees, with no checkout (GH-2725).
func (t *Tracker) HostTree(project, dir string) ([]string, error) {
	var repo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := t.get("repos/"+project, &repo); err != nil {
		return nil, err
	}
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := t.get(fmt.Sprintf("repos/%s/git/trees/%s?recursive=1", project, url.PathEscape(repo.DefaultBranch)), &tree); err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	var out []string
	for _, e := range tree.Tree {
		if e.Type == "blob" && strings.HasPrefix(e.Path, prefix) {
			out = append(out, e.Path)
		}
	}
	return out, nil
}

// HostFile reads a file on the default branch from GitHub's contents and
// names the commit that last changed it from its history.
func (t *Tracker) HostFile(project, path string) ([]byte, string, error) {
	var file struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := t.get(fmt.Sprintf("repos/%s/contents/%s", project, escapePath(path)), &file); err != nil {
		return nil, "", err
	}
	content := []byte(file.Content)
	if file.Encoding == "base64" {
		b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
		if err != nil {
			return nil, "", fmt.Errorf("%s in %s: %w", path, project, err)
		}
		content = b
	}
	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := t.get(fmt.Sprintf("repos/%s/commits?path=%s&per_page=1", project, url.QueryEscape(path)), &commits); err != nil {
		return nil, "", err
	}
	if len(commits) == 0 {
		return nil, "", fmt.Errorf("%s in %s has no commit: %w", path, project, tracker.ErrNotFound)
	}
	return content, commits[0].SHA, nil
}

// escapePath escapes each segment of a repository path for a URL.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
