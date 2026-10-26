// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeTools is go and gh for a publish: go writes a binary naming its
// platform, and gh answers as login and holds a release's assets.
type fakeTools struct {
	login     string
	release   map[string]string // asset name -> sha256 digest; nil means no release
	created   bool
	uploaded  []string
	uploadErr error
}

func (f *fakeTools) run(_ string, env []string, name string, args ...string) (string, error) {
	switch {
	case name == "go":
		out := args[slices.Index(args, "-o")+1]
		// The workspace is a fresh temporary directory each build, which a
		// -trimpath binary does not carry.
		env = slices.DeleteFunc(slices.Clone(env), func(e string) bool { return strings.HasPrefix(e, "GOWORK=") })
		return "", os.WriteFile(out, []byte("agent "+strings.Join(env, " ")), 0o755)
	case name == "gh" && args[0] == "auth":
		return "token", nil
	case name == "gh" && args[0] == "api":
		if !slices.Contains(env, "GH_TOKEN=token") {
			return "", errors.New("gh api without the organization's token")
		}
		return f.login, nil
	case name == "gh" && args[1] == "view":
		if f.release == nil {
			return "", errors.New("gh release view: exit status 1: release not found")
		}
		var assets []map[string]string
		for n, d := range f.release {
			assets = append(assets, map[string]string{"name": n, "digest": "sha256:" + d})
		}
		b, _ := json.Marshal(map[string]any{"assets": assets})
		return string(b), nil
	case name == "gh" && args[1] == "create":
		if !slices.Contains(args, "--verify-tag") {
			return "", errors.New("a release created without --verify-tag")
		}
		f.created, f.release = true, map[string]string{}
		return "", nil
	case name == "gh" && args[1] == "upload":
		if f.uploadErr != nil {
			return "", f.uploadErr
		}
		for _, p := range args[5:] {
			f.uploaded = append(f.uploaded, filepath.Base(p))
		}
		return "", nil
	}
	return "", errors.New("unexpected " + name + " " + strings.Join(args, " "))
}

// tagged is a repository whose HEAD carries tag, pushed to origin.
func tagged(t *testing.T, tag string) (string, Git) {
	t.Helper()
	dir, git := repo(t, gatesMarkdown)
	for _, args := range [][]string{{"tag", "-a", tag, "-m", "Release " + tag}, {"push", "-q", "origin", "refs/tags/" + tag}} {
		if _, err := git(args...); err != nil {
			t.Fatal(err)
		}
	}
	return dir, git
}

func publisher(dir string, git Git, f *fakeTools, out io.Writer) Publisher {
	return Publisher{Root: dir, Git: git, Run: f.run, Out: out, Repo: "org/repo", Account: "org-owner", Core: coreFixture(dir)}
}

// coreFixture lays out the least a release build reads beside an
// application at root: agent-core's go.mod and tools tree, and the
// application's go.mod and profiles. It returns the agent-core directory.
func coreFixture(root string) string {
	core := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-agent-core")
	for path, body := range map[string]string{
		filepath.Join(core, "go.mod"):                                  "module example.com/core\n\ngo 1.26\n",
		filepath.Join(core, "tools", "builtin", "done.yaml"):           "done\n",
		filepath.Join(root, "go.mod"):                                  "module example.com/app\n\ngo 1.26\n",
		filepath.Join(root, "plugin", "agents", "pop", "profile.yaml"): "name: pop\n",
	} {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(body), 0o644)
	}
	return core
}

func sha(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func TestBuildWritesEveryPlatformAndItsSums(t *testing.T) {
	f := &fakeTools{}
	dist := t.TempDir()
	assets, err := publisher(t.TempDir(), nil, f, io.Discard).Build(dist, Platforms)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range assets {
		names = append(names, a.Name)
	}
	want := []string{"agent_linux_amd64", "agent_linux_arm64", "agent_darwin_amd64", "agent_darwin_arm64", ProfilesArchive, SumsFile}
	if !slices.Equal(names, want) {
		t.Fatalf("assets %v, want %v", names, want)
	}
	sums, _ := os.ReadFile(filepath.Join(dist, SumsFile))
	for _, a := range assets[:5] {
		data, _ := os.ReadFile(a.Path)
		if !strings.Contains(string(sums), sha(data)+"  "+a.Name+"\n") {
			t.Fatalf("%s not listed in\n%s", a.Name, sums)
		}
	}
	for _, a := range assets[:4] {
		if data, _ := os.ReadFile(a.Path); !strings.Contains(string(data), "CGO_ENABLED=0") {
			t.Fatalf("%s: %q", a.Name, data)
		}
	}
}

func TestPublishCreatesTheReleaseWithEveryAsset(t *testing.T) {
	dir, git := tagged(t, "v0.20261015.1")
	f := &fakeTools{login: "org-owner"}
	var out bytes.Buffer
	if err := publisher(dir, git, f, &out).Publish(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !f.created || len(f.uploaded) != 6 || !slices.Contains(f.uploaded, SumsFile) {
		t.Fatalf("created %v, uploaded %v", f.created, f.uploaded)
	}
}

func TestPublishCompletesAPartialRelease(t *testing.T) {
	dir, git := tagged(t, "v0.20261015.1")
	f := &fakeTools{login: "org-owner"}
	p := publisher(dir, git, f, io.Discard)
	dist := t.TempDir()
	assets, _ := p.Build(dist, Platforms)
	f.release = map[string]string{assets[0].Name: assets[0].Sum, assets[1].Name: assets[1].Sum}
	if err := p.Publish(); err != nil {
		t.Fatal(err)
	}
	if f.created || len(f.uploaded) != 4 || slices.Contains(f.uploaded, assets[0].Name) {
		t.Fatalf("created %v, uploaded %v", f.created, f.uploaded)
	}

	f.release[assets[2].Name] = sha([]byte("another build"))
	f.uploaded = nil
	if err := p.Publish(); err == nil || !strings.Contains(err.Error(), assets[2].Name) || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("an asset that differs was not refused: %v", err)
	}
	if len(f.uploaded) != 0 {
		t.Fatalf("uploaded %v next to a differing asset", f.uploaded)
	}
}

func TestPublishNamesARerunAfterAFailedUpload(t *testing.T) {
	dir, git := tagged(t, "v0.20261015.1")
	f := &fakeTools{login: "org-owner", uploadErr: errors.New("connection reset")}
	if err := publisher(dir, git, f, io.Discard).Publish(); err == nil || !strings.Contains(err.Error(), "mage publish again") {
		t.Fatalf("a failed upload did not name the rerun: %v", err)
	}
	if tags, _ := git("tag", "--list", "v0.20261015.1"); tags == "" {
		t.Fatal("a failed upload removed the tag")
	}
}

func TestPublishRefusals(t *testing.T) {
	dir, git := repo(t, gatesMarkdown)
	f := &fakeTools{login: "org-owner"}
	if err := publisher(dir, git, f, io.Discard).Publish(); err == nil || !strings.Contains(err.Error(), "carries no") {
		t.Fatalf("an untagged HEAD: %v", err)
	}
	git("tag", "-a", "v0.20261015.1", "-m", "Release")
	if err := publisher(dir, git, f, io.Discard).Publish(); err == nil || !strings.Contains(err.Error(), "not on origin") {
		t.Fatalf("an unpushed tag: %v", err)
	}
	git("push", "-q", "origin", "refs/tags/v0.20261015.1")
	f.login = "a-person"
	if err := publisher(dir, git, f, io.Discard).Publish(); err == nil || !strings.Contains(err.Error(), "only as the organization") {
		t.Fatalf("a person's account: %v", err)
	}
	if f.created || len(f.uploaded) != 0 {
		t.Fatalf("a refused publish created %v, uploaded %v", f.created, f.uploaded)
	}
}

func TestPublishDryRunUploadsNothing(t *testing.T) {
	dir, git := tagged(t, "v0.20261015.1")
	f := &fakeTools{login: "a-person"}
	p := publisher(dir, git, f, io.Discard)
	p.DryRun = true
	if err := p.Publish(); err != nil {
		t.Fatal(err)
	}
	if f.created || len(f.uploaded) != 0 {
		t.Fatalf("a dry run created %v, uploaded %v", f.created, f.uploaded)
	}
	dist, _ := git("rev-parse", "--git-path", "scrum-master-publish/v0.20261015.1")
	if _, err := os.Stat(filepath.Join(dir, dist, SumsFile)); err != nil {
		t.Fatalf("a dry run built no assets: %v", err)
	}
}

func TestTagPublishesOnlyAPushedTag(t *testing.T) {
	dir, git := repo(t, gatesMarkdown)
	run := func(string, string, io.Writer) error { return nil }
	var published []string
	r := release(dir, git, run, io.Discard)
	r.Publish = func(tag string) error { published = append(published, tag); return nil }
	if err := r.Tag(); err != nil || len(published) != 1 {
		t.Fatalf("push auto: published %v, %v", published, err)
	}

	dir, git = repo(t, strings.Replace(gatesMarkdown, "auto", "ask", 1))
	var out bytes.Buffer
	r = release(dir, git, run, &out)
	r.Publish = func(tag string) error { published = append(published, tag); return nil }
	if err := r.Tag(); err != nil || len(published) != 1 || !strings.Contains(out.String(), "run mage publish") {
		t.Fatalf("a declined push: published %v, %v\n%s", published, err, out.String())
	}

	r.Preflight = func() error { return errors.New("gh answers as a-person") }
	if err := r.DryRun(); err == nil || !strings.Contains(err.Error(), "a-person") {
		t.Fatalf("a failed preflight did not stop the release: %v", err)
	}
}
