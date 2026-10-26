// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeGitLab answers the glab calls a GitLab release makes, holding the
// release's links, its SHA256SUMS, and its description.
type fakeGitLab struct {
	user     string
	exists   bool
	links    []string
	sums     string
	notes    string
	uploaded []string
	created  bool
	ran      []string
}

func (f *fakeGitLab) run(_ string, env []string, name string, args ...string) (string, error) {
	f.ran = append(f.ran, name+" "+strings.Join(args, " "))
	switch {
	case name == "go":
		out := args[slices.Index(args, "-o")+1]
		return "", os.WriteFile(out, []byte("agent"), 0o755)
	case name == "glab" && args[0] == "api" && args[1] == "user":
		return fmt.Sprintf(`{"username":%q}`, f.user), nil
	case name == "glab" && args[0] == "api" && args[1] == "--method":
		f.notes = strings.TrimPrefix(args[len(args)-1], "description=")
		return "", nil
	case name == "glab" && args[0] == "api":
		if !strings.HasPrefix(args[1], "projects/group%2Fdeclarative-agents/releases/") {
			return "", fmt.Errorf("glab api %s: not the project's release", args[1])
		}
		if !f.exists {
			return "", errors.New("glab: 404 Not Found")
		}
		var links []map[string]string
		for _, l := range f.links {
			links = append(links, map[string]string{"name": l})
		}
		b, _ := json.Marshal(map[string]any{"description": f.notes, "assets": map[string]any{"links": links}})
		return string(b), nil
	case name == "glab" && args[1] == "create":
		f.created, f.exists = true, true
		return "", nil
	case name == "glab" && args[1] == "upload":
		for _, p := range args[5:] {
			f.uploaded = append(f.uploaded, filepath.Base(p))
			f.links = append(f.links, filepath.Base(p))
		}
		return "", nil
	case name == "glab" && args[1] == "download":
		if f.sums == "" {
			return "", errors.New("no asset SHA256SUMS")
		}
		dir := args[slices.Index(args, "--dir")+1]
		os.MkdirAll(dir, 0o755)
		return "", os.WriteFile(filepath.Join(dir, SumsFile), []byte(f.sums), 0o644)
	}
	return "", fmt.Errorf("unexpected %s %v", name, args)
}

func gitlabPublisher(t *testing.T, f *fakeGitLab) Publisher {
	dir, git := tagged(t, "v0.20261015.1")
	return Publisher{Root: dir, Git: git, Run: f.run, Out: io.Discard, Repo: "group/declarative-agents", Account: "org-owner",
		Core: coreFixture(dir), Forge: ForgeGitLab}
}

// A release on a GitLab fork is created as the organization, with every
// asset uploaded through glab (GH-31).
func TestGitLabPublishCreatesTheRelease(t *testing.T) {
	f := &fakeGitLab{user: "org-owner"}
	if err := gitlabPublisher(t, f).Publish(); err != nil {
		t.Fatal(err)
	}
	if !f.created || len(f.uploaded) != len(Platforms)+2 || !slices.Contains(f.uploaded, SumsFile) || !slices.Contains(f.uploaded, ProfilesArchive) {
		t.Fatalf("created %v, uploaded %v", f.created, f.uploaded)
	}
}

// A GitLab link records no digest: an asset already on the release is kept
// when the release's SHA256SUMS lists this build's checksum, refused when it
// lists another, and refused when the release has no SHA256SUMS (GH-31).
func TestGitLabPublishHoldsAssetsToTheReleasesSums(t *testing.T) {
	f := &fakeGitLab{user: "org-owner"}
	p := gitlabPublisher(t, f)
	assets, err := p.Build(t.TempDir(), Platforms)
	if err != nil {
		t.Fatal(err)
	}
	var sums strings.Builder
	for _, a := range assets {
		fmt.Fprintf(&sums, "%s  %s\n", a.Sum, a.Name)
	}
	f.exists, f.links, f.sums = true, []string{assets[0].Name, SumsFile}, sums.String()
	if err := p.Publish(); err != nil {
		t.Fatal(err)
	}
	if f.created || slices.Contains(f.uploaded, assets[0].Name) || len(f.uploaded) != len(assets)-2 {
		t.Fatalf("created %v, uploaded %v", f.created, f.uploaded)
	}

	f.uploaded, f.links, f.sums = nil, []string{assets[0].Name, SumsFile}, strings.Replace(sums.String(), assets[0].Sum, strings.Repeat("0", 64), 1)
	if err := p.Publish(); err == nil || !strings.Contains(err.Error(), "refusing to replace it") {
		t.Fatalf("a mismatched asset: %v", err)
	}
	f.links, f.sums = []string{assets[0].Name}, ""
	if err := p.Publish(); err == nil || !strings.Contains(err.Error(), "no "+SumsFile) {
		t.Fatalf("an asset with no sums to hold it to: %v", err)
	}
}

// glab answering as anyone but the organization stops the release.
func TestGitLabPublishRefusesAnotherAccount(t *testing.T) {
	f := &fakeGitLab{user: "a-person"}
	if err := gitlabPublisher(t, f).Publish(); err == nil || !strings.Contains(err.Error(), "not org-owner") {
		t.Fatalf("err = %v", err)
	}
	if f.created {
		t.Fatal("a release was created as another account")
	}
}

// The image's pinned reference is added to a GitLab release's description.
func TestGitLabNotesTheImage(t *testing.T) {
	f := &fakeGitLab{user: "org-owner", exists: true, notes: "Release v0.20261015.1"}
	p := gitlabPublisher(t, f)
	h, err := p.host()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.noteDigest("v0.20261015.1", h, "registry.example/group/scrum-master@sha256:abc"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.notes, "Image: registry.example/group/scrum-master@sha256:abc") {
		t.Fatalf("notes = %q", f.notes)
	}
}
