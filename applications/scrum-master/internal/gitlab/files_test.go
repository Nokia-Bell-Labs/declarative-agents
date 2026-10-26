// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package gitlab

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// filesAPI answers the endpoints HostTree and HostFile read, for one
// project whose default branch is trunk.
type filesAPI struct{}

func (filesAPI) Do(method, path string, body any) ([]byte, error) {
	var out any
	switch path {
	case "projects/g%2Fteam/repository/tree?path=status&recursive=true&per_page=100&page=1":
		out = []map[string]any{
			{"path": "status/reports", "type": "tree"},
			{"path": "status/reports/2026-Q4/2026-W44-alice.md", "type": "blob"},
		}
	case "projects/g%2Fteam":
		out = map[string]any{"default_branch": "trunk"}
	case "projects/g%2Fteam/repository/files/status%2Freports%2F2026-Q4%2F2026-W44-alice.md?ref=trunk":
		out = map[string]any{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("---\nauthor: alice\n---\n")), "last_commit_id": "def456"}
	default:
		return nil, errors.New("glab api: 404 Not Found")
	}
	return json.Marshal(out)
}

func (filesAPI) Run(args ...string) ([]byte, error) { return nil, errors.New("no glab") }

// The host serves a project's files on its default branch, and the commit
// that last changed each, with no checkout (GH-2725).
func TestHostFilesReadTheDefaultBranchWithNoCheckout(t *testing.T) {
	tr := New(t.TempDir(), filesAPI{})
	paths, err := tr.HostTree("g/team", "status")
	if err != nil || strings.Join(paths, ",") != "status/reports/2026-Q4/2026-W44-alice.md" {
		t.Fatalf("HostTree = %v, %v", paths, err)
	}
	content, commit, err := tr.HostFile("g/team", paths[0])
	if err != nil || commit != "def456" || !strings.Contains(string(content), "author: alice") {
		t.Fatalf("HostFile = %q, %q, %v", content, commit, err)
	}
	if _, _, err := tr.HostFile("g/team", "status/missing.md"); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("a missing file: %v, want ErrNotFound", err)
	}
}
