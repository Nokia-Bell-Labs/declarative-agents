// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/spectool"
)

// sourceArchive is a release's source as a forge serves it: one top folder
// holding agent-core with its agent command.
func sourceArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"declarative-agents-x/agent-core/go.mod":            "module example.invalid/agent-core\n\ngo 1.26\n",
		"declarative-agents-x/agent-core/cmd/agent/main.go": "package main\n\nfunc main() {}\n",
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// An application with no agent-core beside it resolves the release its
// pins file names, the source fetched from the forge the pins file names,
// and with neither a checkout nor a pin it says what it looked for
// (GH-2754).
func TestResolveFetchesThePinnedRelease(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HOME", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	const tag = "v0.20990101.1"
	archive := sourceArchive(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/archive/refs/tags/"+tag+".tar.gz" {
			http.NotFound(w, r)
			return
		}
		w.Write(archive)
	}))
	defer srv.Close()

	app := t.TempDir()
	os.MkdirAll(filepath.Join(app, "docs"), 0o755)
	if err := os.WriteFile(filepath.Join(app, spectool.PinsFile), []byte("pins:\n  "+spectool.ParentRepository+": "+tag+"\nsources:\n  "+spectool.ParentRepository+": "+srv.URL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := Resolve(app, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != tag {
		t.Errorf("tag = %q, want %s", rel.Tag, tag)
	}
	if _, err := os.Stat(filepath.Join(rel.CoreRoot(), "cmd", "agent", "main.go")); err != nil {
		t.Errorf("the pinned release's agent-core is not laid out: %v", err)
	}

	bare := t.TempDir()
	if _, err := Resolve(bare, nil); err == nil || !strings.Contains(err.Error(), spectool.CheckoutsFile) || !strings.Contains(err.Error(), spectool.PinsFile) {
		t.Fatalf("with no runtime: %v", err)
	}
}

// A parent checkout named in docs/external-checkouts.yaml is the runtime,
// ahead of a pin (GH-2754).
func TestResolvePrefersTheNamedCheckout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	parent := t.TempDir()
	os.MkdirAll(filepath.Join(parent, "agent-core", "cmd", "agent"), 0o755)
	app := t.TempDir()
	os.MkdirAll(filepath.Join(app, "docs"), 0o755)
	os.WriteFile(filepath.Join(app, spectool.CheckoutsFile), []byte("checkouts:\n  "+spectool.ParentRepository+": "+parent+"\n"), 0o644)
	os.WriteFile(filepath.Join(app, spectool.PinsFile), []byte("pins:\n  "+spectool.ParentRepository+": v0.20990101.1\n"), 0o644)
	rel, err := Resolve(app, nil)
	if err != nil || rel.Src != parent || rel.Tag != "checkout" {
		t.Fatalf("Resolve = %+v, %v, want the checkout %s", rel, err, parent)
	}
}

// A pin whose source is a git remote is cloned at its tag, with the
// remote's other releases, no forge's archive URL, and the person's own
// git credentials (GH-2761, GH-2765).
func TestResolveClonesAPinnedGitSource(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HOME", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	const tag = "v0.20990101.2"
	work, bare := t.TempDir(), filepath.Join(t.TempDir(), "declarative-agents.git")
	os.MkdirAll(filepath.Join(work, "agent-core", "cmd", "agent"), 0o755)
	os.WriteFile(filepath.Join(work, "agent-core", "go.mod"), []byte("module example.invalid/agent-core\n"), 0o644)
	os.WriteFile(filepath.Join(work, "agent-core", "cmd", "agent", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	for _, args := range [][]string{
		{"-C", work, "init", "-q", "-b", "main"},
		{"-C", work, "add", "-A"},
		{"-C", work, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "runtime"},
		{"-C", work, "tag", "v0.20990101.1"},
		{"-C", work, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "the next release"},
		{"-C", work, "tag", tag},
		{"clone", "-q", "--bare", work, bare},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	app := t.TempDir()
	os.MkdirAll(filepath.Join(app, "docs"), 0o755)
	os.WriteFile(filepath.Join(app, spectool.PinsFile), []byte("pins:\n  "+spectool.ParentRepository+": "+tag+"\nsources:\n  "+spectool.ParentRepository+": "+bare+"\n"), 0o644)
	rel, err := Resolve(app, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rel.CoreRoot(), "cmd", "agent", "main.go")); err != nil || rel.Tag != tag {
		t.Fatalf("the cloned runtime %+v: %v", rel, err)
	}
	// An earlier release a derived_from entry cites resolves in the clone
	// (GH-2765).
	if out, err := exec.Command("git", "-C", rel.Src, "rev-parse", "--verify", "-q", "refs/tags/v0.20990101.1").CombinedOutput(); err != nil {
		t.Fatalf("the clone lacks the earlier release: %v: %s", err, out)
	}
}
