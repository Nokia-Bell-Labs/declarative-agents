// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The image's build: the Dockerfile at the repository root, the platforms
// it is built for, and the buildx builder that can push them all (eng03).
const (
	Dockerfile     = "Dockerfile"
	imageBuilder   = "scrum-master-publish"
	imageMetadata  = "image-metadata.json"
	imageLatestTag = "latest"
)

// The image's description and license, which the Dockerfile's labels
// repeat on each platform's image.
const (
	ImageDescription = "the agent with the scrum-master words and profiles"
	ImageLicenses    = "BSD-3-Clause"
)

// ImagePlatforms are the platforms the image is built for.
var ImagePlatforms = []Platform{{"linux", "amd64"}, {"linux", "arm64"}}

// ImageBuild is one docker buildx build of the image from the release
// binaries in Dist.
type ImageBuild struct {
	Dist      string
	Platforms []Platform
	Refs      []string
	Version   string
	Revision  string
	// Push pushes the image; otherwise it is loaded into the local docker.
	Push bool
}

// BuildImage builds the image as b says and returns its digest when pushed.
// The SBOM is attached to the image index, and provenance is kept to
// mode=min so it names no machine or person. A pushed image's index carries
// the source, description, and license annotations, which ghcr reads to link
// the package to its repository; a loaded image has no index.
func (p Publisher) BuildImage(b ImageBuild) (string, error) {
	var platforms []string
	for _, pl := range b.Platforms {
		platforms = append(platforms, pl.OS+"/"+pl.Arch)
		if _, err := os.Stat(filepath.Join(b.Dist, AssetName("agent", pl))); err != nil {
			return "", fmt.Errorf("the image needs %s: %w", AssetName("agent", pl), err)
		}
	}
	if _, err := os.Stat(filepath.Join(b.Dist, ProfilesArchive)); err != nil {
		return "", fmt.Errorf("the image needs %s: %w", ProfilesArchive, err)
	}
	metadata := filepath.Join(b.Dist, imageMetadata)
	args := []string{"buildx", "build",
		"--platform", strings.Join(platforms, ","),
		"--file", filepath.Join(p.Root, Dockerfile),
		"--build-arg", "VERSION=" + b.Version,
		"--build-arg", "REVISION=" + b.Revision,
		"--sbom=true", "--provenance=mode=min",
		"--metadata-file", metadata,
	}
	for _, ref := range b.Refs {
		args = append(args, "--tag", ref)
	}
	if b.Push {
		for _, a := range []string{
			"org.opencontainers.image.source=https://github.com/" + p.Repo,
			"org.opencontainers.image.description=" + ImageDescription,
			"org.opencontainers.image.licenses=" + ImageLicenses,
		} {
			args = append(args, "--annotation", "index:"+a)
		}
		args = append(args, "--builder", imageBuilder, "--push")
	} else {
		args = append(args, "--load")
	}
	if _, err := p.Run(p.Root, nil, "docker", append(args, b.Dist)...); err != nil {
		return "", err
	}
	if !b.Push {
		return "", nil
	}
	data, err := os.ReadFile(metadata)
	if err != nil {
		return "", err
	}
	var meta struct {
		Digest string `json:"containerimage.digest"`
	}
	if err := json.Unmarshal(data, &meta); err != nil || meta.Digest == "" {
		return "", fmt.Errorf("docker buildx wrote no image digest to %s: %v", metadata, err)
	}
	return meta.Digest, nil
}

// CheckImageTools fails unless docker buildx, which builds and pushes the
// image, is here.
func (p Publisher) CheckImageTools() error {
	if _, err := p.Run(p.Root, nil, "docker", "buildx", "version"); err != nil {
		return fmt.Errorf("publishing the image needs docker buildx: %w", err)
	}
	return nil
}

// Check is the release preflight: the organization account and docker
// buildx.
func (p Publisher) Check() error {
	if err := p.CheckAccount(); err != nil {
		return err
	}
	return p.CheckImageTools()
}

// PublishImage builds the image for every image platform from the release
// binaries Binaries built, pushes it as ImageRepo:tag and latest, and adds
// its digest to the release's notes, which pipelines pin from. An
// image already at tag is kept. A dry run names the build and runs nothing.
func (p Publisher) PublishImage(tag string) error {
	ref := p.ImageRepo + ":" + tag
	if p.DryRun {
		fmt.Fprintf(p.Out, "publish: would build and push %s for %s and list its digest on the release\n", ref, platformList(ImagePlatforms))
		return nil
	}
	dist, err := p.dist(tag)
	if err != nil {
		return err
	}
	revision, err := p.Git("rev-parse", tag+"^{commit}")
	if err != nil {
		return err
	}
	build := ImageBuild{Dist: dist, Platforms: ImagePlatforms, Refs: []string{ref, p.ImageRepo + ":" + imageLatestTag}, Version: tag, Revision: revision, Push: true}
	if err := p.CheckImageTools(); err != nil {
		return err
	}
	h, err := p.host()
	if err != nil {
		return err
	}
	if err := h.registryLogin(p.ImageRepo); err != nil {
		return err
	}
	digest, err := p.Run(p.Root, nil, "docker", "buildx", "imagetools", "inspect", ref, "--format", "{{.Manifest.Digest}}")
	if err == nil && digest != "" {
		// An image already at the tag is kept only when it is this commit's:
		// a tag the image name shares with another source would otherwise
		// be listed on this release (GH-18).
		label, lerr := p.Run(p.Root, nil, "docker", "buildx", "imagetools", "inspect", ref, "--format", `{{index .Image.Config.Labels "org.opencontainers.image.revision"}}`)
		head, herr := p.Git("rev-parse", "HEAD")
		if lerr == nil && herr == nil && strings.TrimSpace(label) != "" && !strings.Contains(label, strings.TrimSpace(head)) {
			return fmt.Errorf("%s exists but was built from %s, not this commit %s; the image name collides with another source — publish under another image name", ref, strings.TrimSpace(label), strings.TrimSpace(head))
		}
		fmt.Fprintf(p.Out, "publish: kept %s@%s, already pushed\n", ref, digest)
	} else {
		if _, err := p.Run(p.Root, nil, "docker", "buildx", "inspect", imageBuilder); err != nil {
			if _, err := p.Run(p.Root, nil, "docker", "buildx", "create", "--name", imageBuilder, "--driver", "docker-container"); err != nil {
				return err
			}
		}
		if digest, err = p.BuildImage(build); err != nil {
			return fmt.Errorf("build and push %s: %w; run mage publish again to complete it", ref, err)
		}
		fmt.Fprintf(p.Out, "publish: pushed %s@%s\n", ref, digest)
	}
	return p.noteDigest(tag, h, p.ImageRepo+"@"+digest)
}

// noteDigest adds the image's pinned reference to the release's notes, once.
func (p Publisher) noteDigest(tag string, h host, pinned string) error {
	_, _, notes, err := h.release(tag)
	if err != nil {
		return err
	}
	if strings.Contains(notes, pinned) {
		return nil
	}
	file := filepath.Join(os.TempDir(), "scrum-master-notes-"+tag+".md")
	defer os.Remove(file)
	if err := os.WriteFile(file, []byte(notes+"\n\nImage: "+pinned+"\n"), 0o644); err != nil {
		return err
	}
	if err := h.setNotes(tag, file); err != nil {
		return err
	}
	fmt.Fprintf(p.Out, "publish: listed %s on the release %s\n", pinned, tag)
	return nil
}

// dist is where Binaries built tag's release binaries. When the image
// platforms' binaries are missing there, it builds them into a directory of
// their own, so the release's SHA256SUMS is never rewritten.
func (p Publisher) dist(tag string) (string, error) {
	gitDir, err := p.Git("rev-parse", "--git-path", "scrum-master-publish")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(p.Root, gitDir)
	}
	dist := filepath.Join(gitDir, tag)
	for _, pl := range ImagePlatforms {
		if _, err := os.Stat(filepath.Join(dist, AssetName("agent", pl))); errors.Is(err, os.ErrNotExist) {
			dist = filepath.Join(dist, "image")
			_, err := p.Build(dist, ImagePlatforms)
			return dist, err
		}
	}
	return dist, nil
}

func platformList(ps []Platform) string {
	var s []string
	for _, pl := range ps {
		s = append(s, pl.OS+"/"+pl.Arch)
	}
	return strings.Join(s, " and ")
}
