// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package release

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// The forges a release is published on (GH-31): GitHub, through gh, the
// public repository's; and GitLab, through glab, a fork's, such as an
// internal one. The release's assets, notes, and image are the same on
// either.
const (
	ForgeGitHub = "github"
	ForgeGitLab = "gitlab"
)

// host is the forge a Publisher releases on.
type host interface {
	// check fails unless the forge answers as the account releases are
	// made as, so nothing on a release names a person.
	check() error
	// release reads tag's release: whether it exists, its assets, and its
	// notes. An asset's digest is empty when the forge records none.
	release(tag string) (exists bool, assets []publishedAsset, notes string, err error)
	create(tag, notesFile string) error
	upload(tag string, paths []string) error
	setNotes(tag, notesFile string) error
	// registryLogin logs docker in to the registry image lives in.
	registryLogin(image string) error
	// download fetches one asset of tag's release into dir.
	download(tag, asset, dir string) (string, error)
}

func (p Publisher) host() (host, error) {
	switch p.Forge {
	case "", ForgeGitHub:
		return githubHost{p}, nil
	case ForgeGitLab:
		return gitlabHost{p}, nil
	}
	return nil, fmt.Errorf("no forge %q; a release is published on %s or %s", p.Forge, ForgeGitHub, ForgeGitLab)
}

type githubHost struct{ p Publisher }

func (h githubHost) env() ([]string, error) { return h.p.ghEnv() }

func (h githubHost) check() error {
	env, err := h.env()
	if err != nil {
		return err
	}
	login, err := h.p.Run(h.p.Root, env, "gh", "api", "user", "-q", ".login")
	if err != nil {
		return err
	}
	if login != h.p.Account {
		return fmt.Errorf("gh answers as %s, not %s; a release is made only as the organization", login, h.p.Account)
	}
	return nil
}

func (h githubHost) release(tag string) (bool, []publishedAsset, string, error) {
	env, err := h.env()
	if err != nil {
		return false, nil, "", err
	}
	view, err := h.p.Run(h.p.Root, env, "gh", "release", "view", tag, "--repo", h.p.Repo, "--json", "assets,body")
	if err != nil {
		if strings.Contains(err.Error(), "release not found") {
			return false, nil, "", nil
		}
		return false, nil, "", err
	}
	var r struct {
		Assets []publishedAsset `json:"assets"`
		Body   string           `json:"body"`
	}
	if err := json.Unmarshal([]byte(view), &r); err != nil {
		return false, nil, "", fmt.Errorf("gh release view %s: %w", tag, err)
	}
	return true, r.Assets, r.Body, nil
}

func (h githubHost) create(tag, notesFile string) error {
	env, err := h.env()
	if err != nil {
		return err
	}
	_, err = h.p.Run(h.p.Root, env, "gh", "release", "create", tag, "--repo", h.p.Repo, "--verify-tag", "--title", tag, "--notes-file", notesFile)
	return err
}

func (h githubHost) upload(tag string, paths []string) error {
	env, err := h.env()
	if err != nil {
		return err
	}
	_, err = h.p.Run(h.p.Root, env, "gh", append([]string{"release", "upload", tag, "--repo", h.p.Repo}, paths...)...)
	return err
}

func (h githubHost) setNotes(tag, notesFile string) error {
	env, err := h.env()
	if err != nil {
		return err
	}
	_, err = h.p.Run(h.p.Root, env, "gh", "release", "edit", tag, "--repo", h.p.Repo, "--notes-file", notesFile)
	return err
}

func (h githubHost) registryLogin(image string) error {
	env, err := h.env()
	if err != nil {
		return err
	}
	if _, err := h.p.Run(h.p.Root, append(env, "ACCOUNT="+h.p.Account), "sh", "-c",
		`printf %s "$GH_TOKEN" | docker login ghcr.io --username "$ACCOUNT" --password-stdin`); err != nil {
		return fmt.Errorf("log in to ghcr.io as %s: %w", h.p.Account, err)
	}
	return nil
}

func (h githubHost) download(tag, asset, dir string) (string, error) {
	env, err := h.env()
	if err != nil {
		return "", err
	}
	_, err = h.p.Run(h.p.Root, env, "gh", "release", "download", tag, "--repo", h.p.Repo, "--pattern", asset, "--dir", dir, "--clobber")
	return filepath.Join(dir, asset), err
}

// gitlabHost releases on a GitLab project through glab, which reads its
// token from GITLAB_TOKEN or its own login: the organization's, for a fork
// that releases as the organization. A release link records no digest, so
// an asset already on a release is held to that release's SHA256SUMS.
type gitlabHost struct{ p Publisher }

func (h gitlabHost) project() string { return url.PathEscape(h.p.Repo) }

func (h gitlabHost) check() error {
	out, err := h.p.Run(h.p.Root, nil, "glab", "api", "user")
	if err != nil {
		return err
	}
	var u struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal([]byte(out), &u); err != nil {
		return fmt.Errorf("glab api user: %w", err)
	}
	if u.Username != h.p.Account {
		return fmt.Errorf("glab answers as %s, not %s; a release is made only as the organization", u.Username, h.p.Account)
	}
	return nil
}

func (h gitlabHost) release(tag string) (bool, []publishedAsset, string, error) {
	out, err := h.p.Run(h.p.Root, nil, "glab", "api", "projects/"+h.project()+"/releases/"+url.PathEscape(tag))
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, nil, "", nil
		}
		return false, nil, "", err
	}
	var r struct {
		Description string `json:"description"`
		Assets      struct {
			Links []struct {
				Name string `json:"name"`
			} `json:"links"`
		} `json:"assets"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return false, nil, "", fmt.Errorf("glab api release %s: %w", tag, err)
	}
	var assets []publishedAsset
	for _, l := range r.Assets.Links {
		assets = append(assets, publishedAsset{Name: l.Name})
	}
	return true, assets, r.Description, nil
}

func (h gitlabHost) create(tag, notesFile string) error {
	_, err := h.p.Run(h.p.Root, nil, "glab", "release", "create", tag, "--repo", h.p.Repo, "--name", tag, "--notes-file", notesFile, "--no-update")
	return err
}

func (h gitlabHost) upload(tag string, paths []string) error {
	_, err := h.p.Run(h.p.Root, nil, "glab", append([]string{"release", "upload", tag, "--repo", h.p.Repo}, paths...)...)
	return err
}

func (h gitlabHost) setNotes(tag, notesFile string) error {
	b, err := os.ReadFile(notesFile)
	if err != nil {
		return err
	}
	_, err = h.p.Run(h.p.Root, nil, "glab", "api", "--method", "PUT", "projects/"+h.project()+"/releases/"+url.PathEscape(tag),
		"--raw-field", "description="+string(b))
	return err
}

func (h gitlabHost) registryLogin(image string) error {
	registry, _, _ := strings.Cut(image, "/")
	if _, err := h.p.Run(h.p.Root, []string{"ACCOUNT=" + h.p.Account, "REGISTRY=" + registry}, "sh", "-c",
		`printf %s "$GITLAB_TOKEN" | docker login "$REGISTRY" --username "$ACCOUNT" --password-stdin`); err != nil {
		return fmt.Errorf("log in to %s as %s with GITLAB_TOKEN: %w", registry, h.p.Account, err)
	}
	return nil
}

func (h gitlabHost) download(tag, asset, dir string) (string, error) {
	_, err := h.p.Run(h.p.Root, nil, "glab", "release", "download", tag, "--repo", h.p.Repo, "--asset-name", asset, "--dir", dir)
	return filepath.Join(dir, asset), err
}

// publishedSums reads the SHA256SUMS already on tag's release, for a forge
// whose assets carry no digest; ok is false when the release has none.
func publishedSums(h host, tag, dir string) (map[string]string, bool, error) {
	path, err := h.download(tag, SumsFile, dir)
	if err != nil {
		return nil, false, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	defer f.Close()
	sums := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 {
			sums[strings.TrimPrefix(fields[1], "*")] = fields[0]
		}
	}
	return sums, true, sc.Err()
}
