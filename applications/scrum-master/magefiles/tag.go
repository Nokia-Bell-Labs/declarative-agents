// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
)

// The GitHub repository releases are published to, and the
// account they are published as (eng03). A fork that releases elsewhere,
// such as an internal GitLab one, names its forge, project, and account in
// the environment variables below instead (GH-31). No image is published
// unless SCRUM_MASTER_RELEASE_IMAGE names a repository the publisher can
// push to: the image is opt-in (GH-2721).
const (
	releaseRepo    = "Nokia-Bell-Labs/declarative-agents"
	releaseAccount = "petar-djukic"
)

// releaseSetting is the environment's value for name, else fallback.
func releaseSetting(name, fallback string) string {
	if v := os.Getenv(name); v != "" { //nolint:forbidigo // A fork's release settings come from the environment (eng03).
		return v
	}
	return fallback
}

// Tag runs the gates in release-gates.md and, when every gate passes,
// creates the next v0.YYYYMMDD.N tag at HEAD, pushes it as the file says,
// and publishes it once it is on origin (eng03). /gh-release-push calls it
// on main.
func Tag() error {
	r, err := scrumMasterRelease()
	if err != nil {
		return err
	}
	return r.Tag()
}

// TagDryRun runs the same checks and gates and creates no tag.
func TagDryRun() error {
	r, err := scrumMasterRelease()
	if err != nil {
		return err
	}
	return r.DryRun()
}

// Publish publishes the tag at HEAD once it is on origin: the release assets and SHA256SUMS on its release — on GitHub, or the forge SCRUM_MASTER_RELEASE_FORGE names — then, when SCRUM_MASTER_RELEASE_IMAGE names a repository, its image; a part already published is checked and kept, so a rerun completes a partial release (eng03).
func Publish() error {
	p, err := scrumMasterPublisher(false)
	if err != nil {
		return err
	}
	return p.Publish()
}

// PublishDryRun builds the release binaries and SHA256SUMS for the tag at HEAD and names what mage publish would upload and push, creating nothing.
func PublishDryRun() error {
	p, err := scrumMasterPublisher(true)
	if err != nil {
		return err
	}
	tag, err := p.Git("describe", "--tags", "--exact-match", "--match", release.DatePrefix+"*", "HEAD")
	if err != nil {
		tag = "untagged"
	}
	if err := p.Binaries(tag); err != nil {
		return err
	}
	return p.PublishImage(tag)
}

// CrossBuild builds the release — the agent for every platform and the profiles archive — and keeps nothing, so a release fails its gates before tagging when a platform does not build (eng03).
func CrossBuild() error {
	p, err := scrumMasterPublisher(true)
	if err != nil {
		return err
	}
	dist, err := os.MkdirTemp("", "scrum-master-cross-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dist)
	_, err = p.Build(dist, release.Platforms)
	return err
}

func scrumMasterRelease() (release.Release, error) {
	root, err := repositoryRoot()
	if err != nil {
		return release.Release{}, err
	}
	p, err := scrumMasterPublisher(false)
	if err != nil {
		return release.Release{}, err
	}
	return release.Release{
		Root:      root,
		Git:       release.ExecGit(root),
		Exec:      release.ShellExec,
		Out:       os.Stdout,
		Preflight: p.Check,
		Publish:   func(string) error { return p.Publish() },
	}, nil
}

func scrumMasterPublisher(dryRun bool) (release.Publisher, error) {
	root, err := repositoryRoot()
	if err != nil {
		return release.Publisher{}, err
	}
	return release.Publisher{
		Root:      root,
		Git:       release.ExecGit(root),
		Run:       release.ExecCommand,
		Out:       os.Stdout,
		Forge:     releaseSetting("SCRUM_MASTER_RELEASE_FORGE", release.ForgeGitHub),
		Repo:      releaseSetting("SCRUM_MASTER_RELEASE_REPO", releaseRepo),
		Account:   releaseSetting("SCRUM_MASTER_RELEASE_ACCOUNT", releaseAccount),
		DryRun:    dryRun,
		ImageRepo: releaseSetting("SCRUM_MASTER_RELEASE_IMAGE", ""),
	}, nil
}
