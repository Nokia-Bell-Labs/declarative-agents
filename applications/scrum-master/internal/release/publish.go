// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
)

// Platform is one operating system and architecture a release binary is
// built for.
type Platform struct{ OS, Arch string }

// Platforms are the agent's platforms, the ones the runtime publishes
// (eng03).
var Platforms = []Platform{
	{"linux", "amd64"}, {"linux", "arm64"},
	{"darwin", "amd64"}, {"darwin", "arm64"},
}

// AssetName is a release binary's name, the convention the declarative-agents
// releases follow: name_os_arch, with .exe on windows.
func AssetName(name string, p Platform) string {
	n := name + "_" + p.OS + "_" + p.Arch
	if p.OS == "windows" {
		n += ".exe"
	}
	return n
}

// SumsFile lists every release binary's SHA-256.
const SumsFile = "SHA256SUMS"

// Asset is one file on a release: a binary or SHA256SUMS.
type Asset struct {
	Name, Path, Sum string
}

// Command runs name with args in dir, with env added to the process's
// environment, and returns its standard output.
type Command func(dir string, env []string, name string, args ...string) (string, error)

// ExecCommand is Command on this machine.
func ExecCommand(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// Publisher publishes the scrum-master tag at HEAD (eng03): the release
// binaries and SHA256SUMS on the tag's GitHub release, then the image. A part
// already published is checked and kept, so a rerun completes a release a
// failed upload left partial.
type Publisher struct {
	Root    string
	Git     Git
	Run     Command
	Out     io.Writer
	Repo    string // the GitHub repository, owner/name
	Account string // the gh account releases are made as
	DryRun  bool
	// ImageRepo is where the tag's image is pushed, such as
	// ghcr.io/owner/name; empty publishes no image.
	ImageRepo string
	// Core is the agent-core the agent builds from; empty is the runtime
	// runtime.Resolve finds for the application (GH-2754).
	Core string
	// Forge is where the release is published, ForgeGitHub (the default)
	// or ForgeGitLab, as an internal fork publishes (GH-31).
	Forge string
}

// versionPackage is where a release's version is stamped in a module, the
// words' or agent-core's: its internal/version, under the module path its
// go.mod declares, never spelled here (GH-18, GH-2754). Both are nested
// modules of the repository, so their build info never carries its tag.
func versionPackage(module string) (string, error) {
	b, err := os.ReadFile(filepath.Join(module, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(mod) + "/internal/version", nil
		}
	}
	return "", fmt.Errorf("%s/go.mod declares no module", module)
}

// Build cross-builds the release into dist (eng03): the agent, composed
// with this application's words (GH-27), for each platform, and the
// profiles archive (GH-34), with SHA256SUMS over them. The tag's version is
// stamped into the agent: agent-core is a nested module, so its build info
// never carries the repository's tag (GH-18).
func (p Publisher) Build(dist string, platforms []Platform) ([]Asset, error) {
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return nil, err
	}
	core, runtimeTag := p.Core, ""
	if core == "" {
		rel, err := runtime.Resolve(p.Root, nil)
		if err != nil {
			return nil, err
		}
		core, runtimeTag = rel.CoreRoot(), rel.Tag
	}
	comp, err := runtime.Compose(core, p.Root)
	if err != nil {
		return nil, fmt.Errorf("the release builds the agent from %s: %w", core, err)
	}
	defer comp.Cleanup()
	tag, _ := p.releaseTag()
	// The runtime a new corpus pins: this release itself when the agent
	// builds from the agent-core beside the words, else the pinned release
	// (GH-2755).
	switch {
	case runtimeTag == "in-repo":
		runtimeTag = tag
	case !version.IsRelease(runtimeTag):
		runtimeTag = ""
	}
	var assets []Asset
	var sums strings.Builder
	add := func(a Asset) error {
		sum, err := fileSum(a.Path)
		if err != nil {
			return err
		}
		a.Sum = sum
		fmt.Fprintf(&sums, "%s  %s\n", sum, a.Name)
		assets = append(assets, a)
		return nil
	}
	for _, pl := range platforms {
		a := Asset{Name: AssetName("agent", pl), Path: filepath.Join(dist, AssetName("agent", pl))}
		env := append([]string{"CGO_ENABLED=0", "GOOS=" + pl.OS, "GOARCH=" + pl.Arch}, comp.Env...)
		args := append([]string{"build", "-trimpath"}, comp.Flags...)
		if tag != "" {
			pkg, err := versionPackage(core)
			if err != nil {
				return nil, err
			}
			words, err := versionPackage(p.Root)
			if err != nil {
				return nil, err
			}
			flags := "-X " + pkg + ".Version=" + tag + " -X " + words + ".Release=" + tag
			if runtimeTag != "" {
				flags += " -X " + words + ".Runtime=" + runtimeTag
			}
			args = append(args, "-ldflags", flags)
		}
		args = append(args, "-o", a.Path, "./cmd/agent")
		if _, err := p.Run(comp.Dir, env, "go", args...); err != nil {
			return nil, fmt.Errorf("build agent for %s/%s: %w", pl.OS, pl.Arch, err)
		}
		if err := add(a); err != nil {
			return nil, err
		}
	}
	archive := Asset{Name: ProfilesArchive, Path: filepath.Join(dist, ProfilesArchive)}
	release := tag
	if release == "" {
		release = "dev"
	}
	if err := WriteProfilesArchive(archive.Path, filepath.Join(p.Root, "plugin", "agents"), filepath.Join(core, "tools"), release); err != nil {
		return nil, err
	}
	if err := add(archive); err != nil {
		return nil, err
	}
	path := filepath.Join(dist, SumsFile)
	if err := os.WriteFile(path, []byte(sums.String()), 0o644); err != nil {
		return nil, err
	}
	sum, err := fileSum(path)
	if err != nil {
		return nil, err
	}
	return append(assets, Asset{Name: SumsFile, Path: path, Sum: sum}), nil
}

// Publish publishes HEAD's tag once it is on origin: the binaries, then the
// image.
func (p Publisher) Publish() error {
	tag, err := p.headTag()
	if err != nil {
		return err
	}
	if remote, err := p.Git("ls-remote", "--tags", "--refs", "origin", "refs/tags/"+tag); err != nil {
		return err
	} else if remote == "" {
		return fmt.Errorf("%s is not on origin; push it with git push origin %s, then run mage publish", tag, tag)
	}
	if err := p.Binaries(tag); err != nil {
		return err
	}
	return p.PublishImage(tag)
}

// releaseTag is the v0.* tag at HEAD when the Publisher has git, else
// empty: a Build without git stamps nothing.
func (p Publisher) releaseTag() (string, error) {
	if p.Git == nil {
		return "", nil
	}
	return p.headTag()
}

// headTag is the v0.* tag at HEAD.
func (p Publisher) headTag() (string, error) {
	tags, err := p.Git("tag", "--points-at", "HEAD", "--list", DatePrefix+"*")
	if err != nil {
		return "", err
	}
	if tags == "" {
		return "", errors.New("HEAD carries no " + DatePrefix + "* tag; mage publish publishes a tag mage tag created")
	}
	return strings.Fields(tags)[0], nil
}

// CheckAccount fails unless the forge, as the organization, answers as the
// account releases are made as, so nothing on a release names a person.
func (p Publisher) CheckAccount() error {
	h, err := p.host()
	if err != nil {
		return err
	}
	return h.check()
}

func (p Publisher) ghEnv() ([]string, error) {
	token, err := p.Run(p.Root, nil, "gh", "auth", "token", "--user", p.Account)
	if err != nil || token == "" {
		return nil, fmt.Errorf("no gh token for %s: %v", p.Account, err)
	}
	return []string{"GH_TOKEN=" + token}, nil
}

// publishedAsset is an asset as gh release view reports it.
type publishedAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// Binaries builds the release binaries at tag and creates the tag's release
// on the forge with them, uploading only what it lacks and refusing an asset
// already there whose checksum differs. A dry run builds them and names
// what it would create and upload.
func (p Publisher) Binaries(tag string) error {
	gitDir, err := p.Git("rev-parse", "--git-path", "scrum-master-publish")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(p.Root, gitDir)
	}
	dist := filepath.Join(gitDir, tag)
	if err := os.RemoveAll(dist); err != nil {
		return err
	}
	assets, err := p.Build(dist, Platforms)
	if err != nil {
		return err
	}
	fmt.Fprintf(p.Out, "publish: built %d binaries and %s in %s\n", len(assets)-1, SumsFile, dist)
	if p.DryRun {
		for _, a := range assets {
			fmt.Fprintf(p.Out, "publish: would upload %s (%s)\n", a.Name, a.Sum)
		}
		fmt.Fprintf(p.Out, "publish: dry run; no release created for %s\n", tag)
		return nil
	}
	if err := p.CheckAccount(); err != nil {
		return err
	}
	h, err := p.host()
	if err != nil {
		return err
	}
	exists, published, _, err := h.release(tag)
	if err != nil {
		return err
	}
	if !exists {
		notes, err := p.Git("tag", "-l", "--format=%(contents)", tag)
		if err != nil {
			return err
		}
		notesFile := filepath.Join(dist, "notes.md")
		if err := os.WriteFile(notesFile, []byte(notes+"\n"), 0o644); err != nil {
			return err
		}
		if err := h.create(tag, notesFile); err != nil {
			return err
		}
		fmt.Fprintf(p.Out, "publish: created the release %s\n", tag)
	}
	// A forge that records no digest holds an asset already there to the
	// SHA256SUMS on the release; with none, nothing there can be verified.
	var sums map[string]string
	if len(published) > 0 && published[0].Digest == "" {
		got, ok, err := publishedSums(h, tag, filepath.Join(dist, "published"))
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("the release %s holds assets but no %s to verify them by; remove them, then run mage publish again", tag, SumsFile)
		}
		sums = got
	}
	var upload []string
	for _, a := range assets {
		i := slices.IndexFunc(published, func(pa publishedAsset) bool { return pa.Name == a.Name })
		if i < 0 {
			upload = append(upload, a.Path)
			continue
		}
		got := strings.TrimPrefix(published[i].Digest, "sha256:")
		if got == "" {
			got = sums[a.Name]
		}
		if got != a.Sum {
			return fmt.Errorf("%s on the release %s has checksum %s, but this build's is %s; refusing to replace it", a.Name, tag, got, a.Sum)
		}
		fmt.Fprintf(p.Out, "publish: kept %s, already on the release\n", a.Name)
	}
	if len(upload) == 0 {
		fmt.Fprintf(p.Out, "publish: the release %s already holds every binary\n", tag)
	} else {
		if err := h.upload(tag, upload); err != nil {
			return fmt.Errorf("upload to the release %s: %w; it stays a draft; run mage publish again to complete it", tag, err)
		}
		fmt.Fprintf(p.Out, "publish: uploaded %d assets to the release %s\n", len(upload), tag)
	}
	// Only a release holding every asset install.sh downloads is made
	// visible; one missing any stays a draft, naming it (GH-2751).
	_, now, _, err := h.release(tag)
	if err != nil {
		return err
	}
	var missing []string
	for _, a := range assets {
		if !slices.ContainsFunc(now, func(pa publishedAsset) bool { return pa.Name == a.Name }) {
			missing = append(missing, a.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the release %s lacks %s, so it stays a draft; run mage publish again to complete it", tag, strings.Join(missing, ", "))
	}
	if err := h.publish(tag); err != nil {
		return fmt.Errorf("publish the release %s: %w", tag, err)
	}
	return nil
}

func fileSum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:]), nil
}
