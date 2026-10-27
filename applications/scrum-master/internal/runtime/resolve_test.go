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
