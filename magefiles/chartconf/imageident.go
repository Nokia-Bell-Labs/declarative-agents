// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package chartconf

import (
	"regexp"
	"strings"
)

// Canonical local first-party images live under this prefix. Role, component,
// and typed tag are what a `docker image ls` reader has without looking up a
// cluster or a Mage target (srd005 R10, ENG01 C7).
const localImagePrefix = "localhost/declarative-agents/"

const publishedImagePrefix = "ghcr.io/nokia-bell-labs/declarative-agents/"

var localImageRoles = map[string]bool{
	"runtime": true,
	"test":    true,
	"derived": true,
	"cache":   true,
}

var (
	localComponentName = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	untypedHexTag      = regexp.MustCompile(`^[0-9a-f]{12}$`)
	typedGitTag        = regexp.MustCompile(`^git-[0-9a-f]{12}-linux-[a-z0-9]+$`)
	typedRecipeTag     = regexp.MustCompile(`^recipe-[0-9a-f]{12}-linux-[a-z0-9]+$`)
	typedUpstreamTag   = regexp.MustCompile(
		`^upstream-[A-Za-z0-9][A-Za-z0-9._-]*-recipe-[0-9a-f]{12}-linux-[a-z0-9]+$`)
)

// CheckImageGrammar applies R10.1 through R10.3 to one container image.
// Callers that sweep literals pass empty chart, overlay, and resource.
func CheckImageGrammar(image string) []Finding {
	return checkImageGrammar("", "", "", image)
}

// checkImageGrammar applies R10.1 through R10.3 to one container image.
func checkImageGrammar(chart, overlay, resource, image string) []Finding {
	repository, tag, _ := splitImage(image)
	lowered := strings.ToLower(repository)
	lowerTag := strings.ToLower(tag)

	if isKindrigRepository(lowered) {
		return []Finding{{chart, overlay, "R10.3", resource, image,
			"kindrig alias hides upstream ownership"}}
	}
	if lowerTag == "local" {
		return []Finding{{chart, overlay, "R10.3", resource, image,
			"local is a mutable alias, not an identity"}}
	}
	if isSmokeRepository(lowered) {
		return []Finding{{chart, overlay, "R10.3", resource, image,
			"consumer-oriented smoke repository"}}
	}

	if strings.HasPrefix(lowered, localImagePrefix) {
		if !canonicalLocalRepository(lowered) {
			return []Finding{{chart, overlay, "R10.1", resource, image,
				"local first-party image must be localhost/declarative-agents/<role>/<component>"}}
		}
		if untypedHexTag.MatchString(lowerTag) {
			return []Finding{{chart, overlay, "R10.3", resource, image,
				"untyped 12-hex local tag"}}
		}
		if !typedIdentityTag(tag) {
			return []Finding{{chart, overlay, "R10.2", resource, image,
				"local tag must be a typed git-, recipe-, or upstream- identity with a linux-<arch> suffix"}}
		}
		return nil
	}

	if strings.HasPrefix(lowered, "declarative-agents/") {
		if untypedHexTag.MatchString(lowerTag) {
			return []Finding{{chart, overlay, "R10.3", resource, image,
				"untyped 12-hex local tag"}}
		}
		return []Finding{{chart, overlay, "R10.1", resource, image,
			"local first-party image must be localhost/declarative-agents/<role>/<component>"}}
	}

	if isPublishedFirstParty(lowered) {
		return nil
	}
	if hasRegistryHost(lowered) {
		return nil
	}
	return []Finding{{chart, overlay, "R10.1", resource, image,
		"third-party image is not fully qualified"}}
}

func canonicalLocalRepository(lowered string) bool {
	rest := strings.TrimPrefix(lowered, localImagePrefix)
	role, component, ok := strings.Cut(rest, "/")
	if !ok || strings.Contains(component, "/") {
		return false
	}
	return localImageRoles[role] && localComponentName.MatchString(component)
}

func typedIdentityTag(tag string) bool {
	return typedGitTag.MatchString(tag) ||
		typedRecipeTag.MatchString(tag) ||
		typedUpstreamTag.MatchString(tag)
}

func isKindrigRepository(repository string) bool {
	return strings.HasPrefix(repository, "kindrig/") ||
		strings.Contains(repository, "/kindrig/")
}

func isSmokeRepository(repository string) bool {
	name := repository
	if i := strings.LastIndex(repository, "/"); i >= 0 {
		name = repository[i+1:]
	}
	return strings.HasSuffix(name, "-smoke")
}

func isPublishedFirstParty(repository string) bool {
	if strings.HasPrefix(repository, publishedImagePrefix) {
		return true
	}
	if !strings.Contains(repository, "-docker.pkg.dev/") {
		return false
	}
	for _, name := range repositoryImageNames {
		if strings.HasSuffix(repository, "/"+name) {
			return true
		}
	}
	return false
}

// hasRegistryHost reports Docker's fully-qualified-name rule: the first path
// component is a host when it is localhost or contains a dot or a colon.
func hasRegistryHost(repository string) bool {
	host, _, ok := strings.Cut(repository, "/")
	if !ok {
		return false
	}
	return host == "localhost" || strings.ContainsAny(host, ".:")
}
