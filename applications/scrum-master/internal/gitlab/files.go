// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package gitlab

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

var _ tracker.HostFiles = (*Tracker)(nil)

// HostTree lists the files under dir on the project's default branch from
// GitLab's repository tree, with no checkout (GH-2725).
func (t *Tracker) HostTree(project, dir string) ([]string, error) {
	var out []string
	err := t.all(fmt.Sprintf("projects/%s/repository/tree?path=%s&recursive=true", id(project), url.QueryEscape(dir)), func(raw json.RawMessage) error {
		var e struct {
			Path string `json:"path"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		if e.Type == "blob" {
			out = append(out, e.Path)
		}
		return nil
	})
	return out, err
}

// HostFile reads a file on the default branch from GitLab's repository
// files, which name the commit that last changed it.
func (t *Tracker) HostFile(project, path string) ([]byte, string, error) {
	var p struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := t.get("projects/"+id(project), &p); err != nil {
		return nil, "", err
	}
	var file struct {
		Content      string `json:"content"`
		Encoding     string `json:"encoding"`
		LastCommitID string `json:"last_commit_id"`
	}
	if err := t.get(fmt.Sprintf("projects/%s/repository/files/%s?ref=%s", id(project), url.PathEscape(path), url.QueryEscape(p.DefaultBranch)), &file); err != nil {
		return nil, "", err
	}
	content := []byte(file.Content)
	if file.Encoding == "base64" {
		b, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			return nil, "", fmt.Errorf("%s in %s: %w", path, project, err)
		}
		content = b
	}
	return content, file.LastCommitID, nil
}
