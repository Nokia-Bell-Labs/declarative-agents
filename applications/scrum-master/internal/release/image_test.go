// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeImageTools is docker, sh, and gh for an image publish: it records
// each command, and answers as the registry and the release's notes say.
type fakeImageTools struct {
	fakeTools
	ran    []string
	pushed string // the digest at the tag in the registry; empty is none
	// revision is the image's org.opencontainers.image.revision label;
	// empty answers with the commit at HEAD, as this release's image would.
	revision string
	head     func() string
	notes    string
	noBuildx bool
}

func (f *fakeImageTools) run(dir string, env []string, name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.ran = append(f.ran, line)
	switch {
	case name == "docker" && f.noBuildx:
		return "", errors.New("docker: 'buildx' is not a docker command")
	case name == "sh":
		return "", nil
	case name == "docker" && args[1] == "imagetools":
		if f.pushed == "" {
			return "", errors.New("not found")
		}
		if strings.Contains(line, "image.revision") {
			if f.revision != "" {
				return f.revision, nil
			}
			return f.head(), nil
		}
		return f.pushed, nil
	case name == "docker" && args[1] == "build":
		meta := args[slices.Index(args, "--metadata-file")+1]
		f.pushed = "sha256:abc"
		return "", os.WriteFile(meta, []byte(`{"containerimage.digest":"sha256:abc"}`), 0o644)
	case name == "docker":
		return "", nil
	case name == "gh" && len(args) > 1 && args[1] == "view":
		// The image tests' release always exists: its notes are f.notes.
		view := map[string]any{"assets": []any{}}
		if out, err := f.fakeTools.run(dir, env, name, args...); err == nil {
			if err := json.Unmarshal([]byte(out), &view); err != nil {
				return "", err
			}
		}
		view["body"] = f.notes
		b, err := json.Marshal(view)
		return string(b), err
	case name == "gh" && len(args) > 1 && args[1] == "edit":
		b, err := os.ReadFile(args[slices.Index(args, "--notes-file")+1])
		f.notes = string(b)
		return "", err
	}
	return f.fakeTools.run(dir, env, name, args...)
}

func (f *fakeImageTools) count(prefix string) int {
	n := 0
	for _, l := range f.ran {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// imageRelease is a pushed tag ready for its image.
func imageRelease(t *testing.T) (Publisher, *fakeImageTools) {
	t.Helper()
	dir, git := tagged(t, "v0.20261015.1")
	f := &fakeImageTools{fakeTools: fakeTools{login: "org-owner"}, notes: "Release v0.20261015.1"}
	p := publisher(dir, git, &f.fakeTools, io.Discard)
	p.Run, p.ImageRepo = f.run, "ghcr.io/org/tpm"
	f.head = func() string { h, _ := p.Git("rev-parse", "HEAD"); return h }
	return p, f
}

// An image already at the tag but built from another commit — a tag the
// image name shares with another source — is refused, not listed on this
// release (GH-18).
func TestPublishRefusesAnImageFromAnotherCommit(t *testing.T) {
	p, f := imageRelease(t)
	f.pushed, f.revision = "sha256:abc", "0123456789abcdef0123456789abcdef01234567"
	err := p.PublishImage("v0.20261015.1")
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("err = %v, want the collision named", err)
	}
	if f.count("gh release edit") != 0 {
		t.Fatalf("a foreign image was listed on the release:\n%s", strings.Join(f.ran, "\n"))
	}
}

func TestPublishPushesAndListsTheImage(t *testing.T) {
	p, f := imageRelease(t)
	if err := p.Publish(); err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(f.ran, "\n"))
	}
	i := slices.IndexFunc(f.ran, func(l string) bool { return strings.HasPrefix(l, "docker buildx build") })
	if i < 0 {
		t.Fatalf("no image built:\n%s", strings.Join(f.ran, "\n"))
	}
	build := f.ran[i]
	for _, want := range []string{"--platform linux/amd64,linux/arm64", "--sbom=true", "--provenance=mode=min", "--tag ghcr.io/org/tpm:v0.20261015.1", "--tag ghcr.io/org/tpm:latest", "--push", "--build-arg VERSION=v0.20261015.1",
		"--annotation index:org.opencontainers.image.source=https://github.com/org/repo",
		"--annotation index:org.opencontainers.image.description=" + ImageDescription,
		"--annotation index:org.opencontainers.image.licenses=" + ImageLicenses} {
		if !strings.Contains(build, want) {
			t.Errorf("the build lacks %q: %s", want, build)
		}
	}
	if !strings.Contains(f.notes, "Image: ghcr.io/org/tpm@sha256:abc") {
		t.Fatalf("the release notes do not list the digest: %q", f.notes)
	}
	if f.count("sh -c") != 1 {
		t.Fatalf("ghcr.io was not logged in to once:\n%s", strings.Join(f.ran, "\n"))
	}
}

func TestPublishKeepsAPushedImage(t *testing.T) {
	p, f := imageRelease(t)
	f.pushed = "sha256:abc"
	if err := p.PublishImage("v0.20261015.1"); err != nil {
		t.Fatal(err)
	}
	if f.count("docker buildx build") != 0 || f.count("gh release edit") != 1 {
		t.Fatalf("a pushed image was rebuilt or not listed:\n%s", strings.Join(f.ran, "\n"))
	}
	f.ran = nil
	if err := p.PublishImage("v0.20261015.1"); err != nil {
		t.Fatal(err)
	}
	if f.count("gh release edit") != 0 {
		t.Fatalf("a listed image was listed again:\n%s", strings.Join(f.ran, "\n"))
	}
}

func TestImageNeedsBuildx(t *testing.T) {
	p, f := imageRelease(t)
	f.noBuildx = true
	if err := p.Check(); err == nil || !strings.Contains(err.Error(), "docker buildx") {
		t.Fatalf("the preflight did not name docker buildx: %v", err)
	}
	if err := p.PublishImage("v0.20261015.1"); err == nil || f.count("docker buildx build") != 0 {
		t.Fatalf("an image was built without its tools: %v", err)
	}
}

// With no image repository named, publish uploads the assets, runs no
// docker, and says no image is published; the preflight asks for no
// buildx (GH-2721).
func TestPublishWithNoImageRepositoryPublishesNoImage(t *testing.T) {
	p, f := imageRelease(t)
	p.ImageRepo, f.noBuildx = "", true
	var out bytes.Buffer
	p.Out = &out
	if err := p.Check(); err != nil {
		t.Fatalf("the preflight wanted the image's tools with no image to publish: %v", err)
	}
	if err := p.Publish(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(f.uploaded) != 6 {
		t.Fatalf("uploaded %v, want every asset", f.uploaded)
	}
	if f.count("docker") != 0 {
		t.Fatalf("docker ran with no image to publish:\n%s", strings.Join(f.ran, "\n"))
	}
	if !strings.Contains(out.String(), NoImage) {
		t.Fatalf("publish did not say no image is published:\n%s", out.String())
	}
}

func TestImageDryRunRunsNothing(t *testing.T) {
	p, f := imageRelease(t)
	p.DryRun = true
	if err := p.Publish(); err != nil {
		t.Fatal(err)
	}
	if f.count("docker") != 0 || f.count("sh") != 0 {
		t.Fatalf("a dry run ran:\n%s", strings.Join(f.ran, "\n"))
	}
}

func TestLoadedImageCarriesNoIndexAnnotation(t *testing.T) {
	dist := t.TempDir()
	platform := Platform{"linux", "arm64"}
	os.WriteFile(filepath.Join(dist, AssetName("agent", platform)), []byte("agent"), 0o755)
	os.WriteFile(filepath.Join(dist, ProfilesArchive), []byte("profiles"), 0o644)
	var ran string
	p := Publisher{Root: t.TempDir(), Repo: "org/repo", Run: func(_ string, _ []string, name string, args ...string) (string, error) {
		ran = name + " " + strings.Join(args, " ")
		return "", nil
	}}
	if _, err := p.BuildImage(ImageBuild{Dist: dist, Platforms: []Platform{platform}, Refs: []string{"scrum-master:test"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ran, "--load") || strings.Contains(ran, "index:") {
		t.Fatalf("a loaded image was built with %s", ran)
	}
}

func TestDockerfileLabelsMatchTheIndexAnnotations(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", Dockerfile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`org.opencontainers.image.description="` + ImageDescription + `"`,
		`org.opencontainers.image.licenses="` + ImageLicenses + `"`,
		`org.opencontainers.image.source="https://github.com/Nokia-Bell-Labs/declarative-agents"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the Dockerfile's labels lack %s", want)
		}
	}
}
