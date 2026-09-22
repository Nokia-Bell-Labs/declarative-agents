// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"fmt"
	"regexp"
	"strings"
)

// Local image references this checkout builds for a cluster. Registry,
// namespace, role, component, identity type, and platform are all in the
// string a `docker image ls` reader sees (ENG01 C7, GH-2511).
const (
	LocalRegistry  = "localhost"
	LocalNamespace = "declarative-agents"
	localPrefix    = LocalRegistry + "/" + LocalNamespace + "/"

	DockerHubRegistry = "docker.io"
	dockerHubIndex    = "index.docker.io"
	dockerHubLibrary  = "library"
)

// ImageRole is one of the four local first-party roles ENG01 names.
type ImageRole string

const (
	RuntimeRole ImageRole = "runtime"
	TestRole    ImageRole = "test"
	DerivedRole ImageRole = "derived"
	CacheRole   ImageRole = "cache"
)

// IdentityType is the typed prefix of a local tag.
type IdentityType string

const (
	GitIdentity      IdentityType = "git"
	RecipeIdentity   IdentityType = "recipe"
	UpstreamIdentity IdentityType = "upstream"
)

var (
	imageRoles = map[ImageRole]bool{
		RuntimeRole: true, TestRole: true, DerivedRole: true, CacheRole: true,
	}
	componentName = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	hex12         = regexp.MustCompile(`^[0-9a-f]{12}$`)
	hexRevision   = regexp.MustCompile(`^[0-9a-fA-F]{12,64}$`)
	digestSHA256  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	upstreamVer   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	linuxArch     = regexp.MustCompile(`^[a-z0-9]+$`)
)

// LocalImage is a canonical localhost/declarative-agents reference.
type LocalImage struct {
	Role            ImageRole
	Component       string
	Type            IdentityType
	Revision        string
	Recipe          string
	UpstreamVersion string
	OS              string
	Arch            string
}

// UpstreamImage is a fully qualified registry reference. Tag names the
// version a reader recognizes; Digest, when set, is the content pin.
type UpstreamImage struct {
	Registry string
	Path     string
	Tag      string
	Digest   string
}

// FormatLocal renders a validated local reference.
func FormatLocal(image LocalImage) (string, error) {
	normalized, err := normalizeLocal(image)
	if err != nil {
		return "", err
	}
	return normalized.String(), nil
}

// ParseLocal reads a canonical local reference.
func ParseLocal(ref string) (LocalImage, error) {
	ref = strings.TrimSpace(ref)
	name, tag, digest := splitReference(ref)
	if digest != "" {
		return LocalImage{}, fmt.Errorf("local image %q carries a digest; typed tags are identity, not content proof", ref)
	}
	lowered := strings.ToLower(name)
	if !strings.HasPrefix(lowered, localPrefix) {
		return LocalImage{}, fmt.Errorf("local image %q must start with %s", ref, localPrefix)
	}
	rest := strings.TrimPrefix(lowered, localPrefix)
	role, component, ok := strings.Cut(rest, "/")
	if !ok || component == "" || strings.Contains(component, "/") {
		return LocalImage{}, fmt.Errorf("local image %q must be %s<role>/<component>:<typed-identity>", ref, localPrefix)
	}
	osName, arch, identity, revision, recipe, version, err := parseTypedTag(tag)
	if err != nil {
		return LocalImage{}, fmt.Errorf("local image %q: %w", ref, err)
	}
	image := LocalImage{
		Role: ImageRole(role), Component: component, Type: identity,
		Revision: revision, Recipe: recipe, UpstreamVersion: version,
		OS: osName, Arch: arch,
	}
	if err := image.validate(); err != nil {
		return LocalImage{}, fmt.Errorf("local image %q: %w", ref, err)
	}
	return image, nil
}

// String is the canonical local reference.
func (image LocalImage) String() string {
	return localPrefix + string(image.Role) + "/" + image.Component + ":" + image.tag()
}

func (image LocalImage) tag() string {
	platform := image.OS + "-" + image.Arch
	switch image.Type {
	case GitIdentity:
		return "git-" + image.Revision + "-" + platform
	case RecipeIdentity:
		return "recipe-" + image.Recipe + "-" + platform
	case UpstreamIdentity:
		return "upstream-" + image.UpstreamVersion + "-recipe-" + image.Recipe + "-" + platform
	}
	return ""
}

func normalizeLocal(image LocalImage) (LocalImage, error) {
	image.Role = ImageRole(strings.ToLower(string(image.Role)))
	image.Component = strings.ToLower(strings.TrimSpace(image.Component))
	image.Type = IdentityType(strings.ToLower(string(image.Type)))
	image.OS = strings.ToLower(strings.TrimSpace(image.OS))
	image.Arch = strings.ToLower(strings.TrimSpace(image.Arch))
	image.UpstreamVersion = strings.TrimSpace(image.UpstreamVersion)
	revision, err := shortenHex("revision", image.Revision)
	if err != nil {
		return LocalImage{}, err
	}
	recipe, err := shortenHex("recipe", image.Recipe)
	if err != nil {
		return LocalImage{}, err
	}
	image.Revision, image.Recipe = revision, recipe
	if image.OS == "" || image.Arch == "" {
		if osName, arch, err := SplitPlatform(image.OS + "/" + image.Arch); err == nil {
			image.OS, image.Arch = osName, arch
		}
	}
	if err := image.validate(); err != nil {
		return LocalImage{}, err
	}
	return image, nil
}

func (image LocalImage) validate() error {
	if !imageRoles[image.Role] {
		return fmt.Errorf("role %q is not runtime, test, derived, or cache", image.Role)
	}
	if !componentName.MatchString(image.Component) {
		return fmt.Errorf("component %q must be a lowercase DNS label", image.Component)
	}
	if strings.HasSuffix(image.Component, "-smoke") {
		return fmt.Errorf("component %q is a consumer-oriented smoke name", image.Component)
	}
	if image.OS != "linux" {
		return fmt.Errorf("os %q must be linux", image.OS)
	}
	if !linuxArch.MatchString(image.Arch) {
		return fmt.Errorf("architecture %q is not a linux arch", image.Arch)
	}
	switch image.Type {
	case GitIdentity:
		if !hex12.MatchString(image.Revision) {
			return fmt.Errorf("git revision must be 12 hexadecimal characters")
		}
		if image.Recipe != "" || image.UpstreamVersion != "" {
			return fmt.Errorf("git identity carries only a revision")
		}
	case RecipeIdentity:
		if !hex12.MatchString(image.Recipe) {
			return fmt.Errorf("recipe hash must be 12 hexadecimal characters")
		}
		if image.Revision != "" || image.UpstreamVersion != "" {
			return fmt.Errorf("recipe identity carries only a recipe hash")
		}
	case UpstreamIdentity:
		if !hex12.MatchString(image.Recipe) {
			return fmt.Errorf("recipe hash must be 12 hexadecimal characters")
		}
		if image.Revision != "" {
			return fmt.Errorf("upstream identity does not carry a git revision")
		}
		if !upstreamVer.MatchString(image.UpstreamVersion) {
			return fmt.Errorf("upstream version %q is not a typed version token", image.UpstreamVersion)
		}
		if strings.Contains(image.UpstreamVersion, "-recipe-") {
			return fmt.Errorf("upstream version %q collides with the recipe delimiter", image.UpstreamVersion)
		}
	default:
		return fmt.Errorf("identity type %q is not git, recipe, or upstream", image.Type)
	}
	return nil
}

func parseTypedTag(tag string) (osName, arch string, identity IdentityType, revision, recipe, version string, err error) {
	if tag == "" {
		return "", "", "", "", "", "", fmt.Errorf("missing typed identity tag")
	}
	if tag == "local" || tag == "latest" {
		return "", "", "", "", "", "", fmt.Errorf("tag %q is mutable, not an identity", tag)
	}
	if hex12.MatchString(tag) {
		return "", "", "", "", "", "", fmt.Errorf("untyped 12-hex tag %q is not an identity", tag)
	}
	platform, found := "", false
	switch {
	case strings.HasPrefix(tag, "git-"):
		identity = GitIdentity
		rest := strings.TrimPrefix(tag, "git-")
		revision, platform, found = strings.Cut(rest, "-")
	case strings.HasPrefix(tag, "recipe-"):
		identity = RecipeIdentity
		rest := strings.TrimPrefix(tag, "recipe-")
		recipe, platform, found = strings.Cut(rest, "-")
	case strings.HasPrefix(tag, "upstream-"):
		identity = UpstreamIdentity
		rest := strings.TrimPrefix(tag, "upstream-")
		version, rest, found = strings.Cut(rest, "-recipe-")
		if !found {
			return "", "", "", "", "", "", fmt.Errorf("upstream tag %q must contain -recipe-<hash>-<os>-<arch>", tag)
		}
		recipe, platform, found = strings.Cut(rest, "-")
	default:
		return "", "", "", "", "", "", fmt.Errorf("tag %q must start with git-, recipe-, or upstream-", tag)
	}
	if !found {
		return "", "", "", "", "", "", fmt.Errorf("tag %q is missing the linux-<arch> suffix", tag)
	}
	osName, arch, err = SplitPlatform(platform)
	return osName, arch, identity, strings.ToLower(revision), strings.ToLower(recipe), version, err
}

func shortenHex(field, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "sha256:") {
		value = strings.TrimPrefix(value, "sha256:")
	}
	if !hexRevision.MatchString(value) {
		return "", fmt.Errorf("%s %q must be 12-64 hexadecimal characters", field, value)
	}
	return value[:12], nil
}

// SplitPlatform reads linux/<arch> or linux-<arch>.
func SplitPlatform(platform string) (osName, arch string, err error) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	switch {
	case strings.Contains(platform, "/"):
		osName, arch, _ = strings.Cut(platform, "/")
	case strings.Contains(platform, "-"):
		osName, arch, _ = strings.Cut(platform, "-")
	default:
		return "", "", fmt.Errorf("platform %q must be linux/<arch>", platform)
	}
	if osName != "linux" || !linuxArch.MatchString(arch) || strings.Contains(arch, "/") || strings.Contains(arch, "-") {
		return "", "", fmt.Errorf("platform %q must be linux/<arch>", platform)
	}
	return osName, arch, nil
}

// FormatGitLocal is the runtime/test git identity helper later units call.
func FormatGitLocal(role ImageRole, component, revision, platform string) (string, error) {
	osName, arch, err := SplitPlatform(platform)
	if err != nil {
		return "", err
	}
	return FormatLocal(LocalImage{
		Role: role, Component: component, Type: GitIdentity,
		Revision: revision, OS: osName, Arch: arch,
	})
}

// FormatRecipeLocal is the cache identity helper later units call.
func FormatRecipeLocal(role ImageRole, component, recipe, platform string) (string, error) {
	osName, arch, err := SplitPlatform(platform)
	if err != nil {
		return "", err
	}
	return FormatLocal(LocalImage{
		Role: role, Component: component, Type: RecipeIdentity,
		Recipe: recipe, OS: osName, Arch: arch,
	})
}

// FormatDerivedLocal is the derived-upstream identity helper later units call.
func FormatDerivedLocal(component, upstreamVersion, recipe, platform string) (string, error) {
	osName, arch, err := SplitPlatform(platform)
	if err != nil {
		return "", err
	}
	return FormatLocal(LocalImage{
		Role: DerivedRole, Component: component, Type: UpstreamIdentity,
		UpstreamVersion: upstreamVersion, Recipe: recipe, OS: osName, Arch: arch,
	})
}

// ParseUpstream reads an upstream reference and canonicalizes Docker Hub
// short names. Digest and tag are preserved.
func ParseUpstream(ref string) (UpstreamImage, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return UpstreamImage{}, fmt.Errorf("upstream image reference is empty")
	}
	name, tag, digest := splitReference(ref)
	lowered := strings.ToLower(name)
	if strings.HasPrefix(lowered, localPrefix) {
		return UpstreamImage{}, fmt.Errorf("image %q is a local first-party reference; use ParseLocal", ref)
	}
	if strings.HasPrefix(lowered, "declarative-agents/") {
		return UpstreamImage{}, fmt.Errorf("image %q is an unprefixed local repository; use %s<role>/<component>:<typed-identity>",
			ref, localPrefix)
	}
	if isKindrigName(lowered) {
		return UpstreamImage{}, fmt.Errorf("image %q uses a kindrig alias that hides upstream ownership", ref)
	}
	if lastComponent(lowered) != "" && strings.HasSuffix(lastComponent(lowered), "-smoke") {
		return UpstreamImage{}, fmt.Errorf("image %q is a consumer-oriented smoke repository", ref)
	}
	tag = strings.TrimSpace(tag)
	digest = strings.ToLower(strings.TrimSpace(digest))
	if tag == "" && digest == "" {
		return UpstreamImage{}, fmt.Errorf("image %q carries no tag", ref)
	}
	if tag == "latest" || tag == "local" {
		return UpstreamImage{}, fmt.Errorf("image %q tag %q is mutable, not an identity", ref, tag)
	}
	if digest != "" && !digestSHA256.MatchString(digest) {
		return UpstreamImage{}, fmt.Errorf("image %q digest is not sha256:<64-hex>", ref)
	}
	registry, path, err := splitRegistryPath(lowered)
	if err != nil {
		return UpstreamImage{}, fmt.Errorf("image %q: %w", ref, err)
	}
	return UpstreamImage{Registry: registry, Path: path, Tag: tag, Digest: digest}, nil
}

// NormalizeUpstream returns the canonical String of ParseUpstream.
func NormalizeUpstream(ref string) (string, error) {
	image, err := ParseUpstream(ref)
	if err != nil {
		return "", err
	}
	return image.String(), nil
}

// String is registry/path:tag[@digest].
func (image UpstreamImage) String() string {
	ref := image.Registry + "/" + image.Path
	if image.Tag != "" {
		ref += ":" + image.Tag
	}
	if image.Digest != "" {
		ref += "@" + image.Digest
	}
	return ref
}

func splitRegistryPath(name string) (registry, path string, err error) {
	host, rest, ok := strings.Cut(name, "/")
	if !ok {
		return DockerHubRegistry, dockerHubLibrary + "/" + name, nil
	}
	if isRegistryHost(host) {
		if host == dockerHubIndex {
			host = DockerHubRegistry
		}
		if host == DockerHubRegistry && !strings.Contains(rest, "/") {
			rest = dockerHubLibrary + "/" + rest
		}
		if rest == "" {
			return "", "", fmt.Errorf("missing repository path")
		}
		return host, rest, nil
	}
	return DockerHubRegistry, name, nil
}

func isRegistryHost(host string) bool {
	return host == "localhost" || strings.ContainsAny(host, ".:")
}

func isKindrigName(name string) bool {
	return strings.HasPrefix(name, "kindrig/") || strings.Contains(name, "/kindrig/")
}

func lastComponent(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// splitReference separates repository, tag, and digest. A registry host
// carrying a port contains a colon before the first slash, so the tag is
// looked for after the last slash only.
func splitReference(image string) (repository, tag, digest string) {
	remainder := image
	if at := strings.Index(remainder, "@"); at >= 0 {
		digest = remainder[at+1:]
		remainder = remainder[:at]
	}
	lastSlash := strings.LastIndex(remainder, "/")
	if colon := strings.LastIndex(remainder, ":"); colon > lastSlash {
		tag = remainder[colon+1:]
		remainder = remainder[:colon]
	}
	return remainder, tag, digest
}
