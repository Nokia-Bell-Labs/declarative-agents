// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package github

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// filesAPI answers the endpoints HostTree and HostFile read, for one
// repository whose default branch is trunk.
type filesAPI struct{ calls []string }

func (f *filesAPI) Do(method, path string, body any) ([]byte, error) {
	f.calls = append(f.calls, path)
	var out any
	switch path {
	case "repos/g/team":
		out = map[string]any{"default_branch": "trunk"}
	case "repos/g/team/git/trees/trunk?recursive=1":
		out = map[string]any{"tree": []map[string]any{
			{"path": "status", "type": "tree"},
			{"path": "status/reports/2026-Q4/2026-W44-alice.md", "type": "blob"},
			{"path": "docs/README.md", "type": "blob"},
		}}
	case "repos/g/team/contents/status/reports/2026-Q4/2026-W44-alice.md":
		out = map[string]any{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("---\nauthor: alice\n---\n"))}
	case "repos/g/team/commits?path=status%2Freports%2F2026-Q4%2F2026-W44-alice.md&per_page=1":
		out = []map[string]any{{"sha": "abc123"}}
	default:
		return nil, errors.New("gh api: HTTP 404: Not Found")
	}
	return json.Marshal(out)
}

func (f *filesAPI) Run(args ...string) ([]byte, error) { return nil, errors.New("no gh") }

// The host serves a repository's files on its default branch, and the
// commit that last changed each, with no checkout (GH-2725).
func TestHostFilesReadTheDefaultBranchWithNoCheckout(t *testing.T) {
	api := &filesAPI{}
	tr := New(t.TempDir(), api, nil)
	paths, err := tr.HostTree("g/team", "status")
	if err != nil || strings.Join(paths, ",") != "status/reports/2026-Q4/2026-W44-alice.md" {
		t.Fatalf("HostTree = %v, %v", paths, err)
	}
	content, commit, err := tr.HostFile("g/team", paths[0])
	if err != nil || commit != "abc123" || !strings.Contains(string(content), "author: alice") {
		t.Fatalf("HostFile = %q, %q, %v", content, commit, err)
	}
	if _, _, err := tr.HostFile("g/team", "status/missing.md"); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("a missing file: %v, want ErrNotFound", err)
	}
}
