// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package pinsurvey resolves the repository's pinned upstream dependencies
// against their registries and reports two properties per pin: whether the
// recorded digest is still what the tag resolves to, and whether a newer
// release exists (srd006-pin-currency).
//
// The client speaks the OCI distribution API over HTTP rather than shelling
// out to a container engine, so the survey needs no daemon and every case is
// testable against an httptest server.
package pinsurvey

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// defaultRegistry serves an image reference that names no registry host.
const defaultRegistry = "registry-1.docker.io"

// defaultNamespace holds Docker Hub's official images, which a reference
// names without any namespace at all: busybox is library/busybox.
const defaultNamespace = "library"

// hubAliases are the names a reference uses for Docker Hub that do not serve
// the distribution API. docker.io serves the website; the API lives at
// registry-1.docker.io, and a reference written either way means the same
// image.
var hubAliases = map[string]string{
	"docker.io":       defaultRegistry,
	"index.docker.io": defaultRegistry,
}

// registryHost resolves a reference's registry to the host serving its API.
func registryHost(name string) string {
	if resolved, ok := hubAliases[name]; ok {
		return resolved
	}
	return name
}

// manifestAccept lists the manifest media types a resolve will take. Both
// index types come first so a multi-architecture image answers with its
// index digest, which is what a chart pins: a per-architecture digest would
// install on one architecture and fail on the other.
var manifestAccept = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

// Reference is an image reference split into the parts a registry call needs.
type Reference struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
	// Architecture selects one platform's manifest out of an index. A chart
	// pins the index digest, which installs on either architecture; the rig
	// pins per-architecture digests, because it pulls and retags for the
	// host it runs on. Empty means the index digest.
	Architecture string
}

// String renders the reference the way a values file or a Go constant holds
// it, so a report quotes what the reader will search for.
func (r Reference) String() string {
	image := r.Repository
	if r.Registry != "" && r.Registry != defaultRegistry {
		image = r.Registry + "/" + r.Repository
	}
	if r.Tag != "" {
		image += ":" + r.Tag
	}
	if r.Digest != "" {
		image += "@" + r.Digest
	}
	return image
}

// ParseReference splits an image reference. A registry host is the first
// segment only when it carries a dot or a colon or is localhost; otherwise
// the segment is part of the repository, which is what distinguishes
// ollama/ollama from ghcr.io/owner/image.
func ParseReference(image string) (Reference, error) {
	remainder := strings.TrimSpace(image)
	if remainder == "" {
		return Reference{}, fmt.Errorf("empty image reference")
	}
	var reference Reference
	if at := strings.Index(remainder, "@"); at >= 0 {
		reference.Digest = remainder[at+1:]
		remainder = remainder[:at]
	}
	lastSlash := strings.LastIndex(remainder, "/")
	if colon := strings.LastIndex(remainder, ":"); colon > lastSlash {
		reference.Tag = remainder[colon+1:]
		remainder = remainder[:colon]
	}
	reference.Registry = defaultRegistry
	if first, rest, found := strings.Cut(remainder, "/"); found {
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			reference.Registry = registryHost(first)
			remainder = rest
		}
	}
	if reference.Registry == defaultRegistry && !strings.Contains(remainder, "/") {
		remainder = defaultNamespace + "/" + remainder
	}
	reference.Repository = remainder
	if reference.Repository == "" {
		return Reference{}, fmt.Errorf("image reference %q names no repository", image)
	}
	return reference, nil
}

// Client resolves references against registries.
type Client struct {
	HTTP *http.Client
	// BaseURL overrides the scheme and host for every registry, so a test
	// points the whole client at one httptest server.
	BaseURL string
	tokens  map[string]string
}

// NewClient returns a client with a bounded timeout. A survey waits on a
// registry it does not control, and an operator target that hangs is worse
// than one that reports unknown.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}, tokens: map[string]string{}}
}

func (c *Client) endpoint(registry, path string) string {
	if c.BaseURL != "" {
		return strings.TrimSuffix(c.BaseURL, "/") + path
	}
	return "https://" + registry + path
}

// do issues a registry request, answering one authentication challenge. A
// registry that challenges twice for the same request is refused rather than
// retried, so a misconfigured realm cannot spin.
func (c *Client) do(registry, path, accept string) (*http.Response, error) {
	request, err := c.request(registry, path, accept)
	if err != nil {
		return nil, err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusUnauthorized {
		return response, nil
	}
	challenge := response.Header.Get("WWW-Authenticate")
	_ = response.Body.Close()
	token, err := c.fetchToken(challenge)
	if err != nil {
		return nil, err
	}
	if c.tokens == nil {
		c.tokens = map[string]string{}
	}
	c.tokens[registry] = token
	request, err = c.request(registry, path, accept)
	if err != nil {
		return nil, err
	}
	return c.HTTP.Do(request)
}

func (c *Client) request(registry, path, accept string) (*http.Request, error) {
	request, err := http.NewRequest(http.MethodGet, c.endpoint(registry, path), nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	if token := c.tokens[registry]; token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request, nil
}

var challengeField = regexp.MustCompile(`(\w+)="([^"]*)"`)

// fetchToken answers a Bearer challenge. This is the standard flow, so one
// implementation covers docker.io, ghcr.io, registry.k8s.io and quay.io
// without a case per registry.
func (c *Client) fetchToken(challenge string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", fmt.Errorf("unsupported authentication challenge %q", challenge)
	}
	fields := map[string]string{}
	for _, match := range challengeField.FindAllStringSubmatch(challenge, -1) {
		fields[match[1]] = match[2]
	}
	realm := fields["realm"]
	if realm == "" {
		return "", fmt.Errorf("authentication challenge names no realm")
	}
	if c.BaseURL != "" {
		if parsed, err := url.Parse(realm); err == nil {
			realm = strings.TrimSuffix(c.BaseURL, "/") + parsed.Path
		}
	}
	query := url.Values{}
	for _, name := range []string{"service", "scope"} {
		if value := fields[name]; value != "" {
			query.Set(name, value)
		}
	}
	if encoded := query.Encode(); encoded != "" {
		realm += "?" + encoded
	}
	response, err := c.HTTP.Get(realm)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request: %s", response.Status)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	if body.Token != "" {
		return body.Token, nil
	}
	if body.AccessToken != "" {
		return body.AccessToken, nil
	}
	return "", fmt.Errorf("token response carries no token")
}

// ResolveDigest returns the digest a tag resolves to now: the index digest,
// or one platform's digest when the reference names an architecture.
func (c *Client) ResolveDigest(reference Reference) (string, error) {
	path := "/v2/" + reference.Repository + "/manifests/" + reference.Tag
	response, err := c.do(reference.Registry, path, manifestAccept)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolve %s: %s", reference, response.Status)
	}
	if reference.Architecture != "" {
		return platformDigest(response.Body, reference)
	}
	if digest := response.Header.Get("Docker-Content-Digest"); digest != "" {
		return digest, nil
	}
	return "", fmt.Errorf("resolve %s: registry returned no content digest", reference)
}

// platformDigest reads one architecture's manifest digest out of an index.
func platformDigest(body io.Reader, reference Reference) (string, error) {
	var index struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.NewDecoder(body).Decode(&index); err != nil {
		return "", fmt.Errorf("decode index for %s: %w", reference, err)
	}
	for _, manifest := range index.Manifests {
		if manifest.Platform.OS == "linux" && manifest.Platform.Architecture == reference.Architecture {
			return manifest.Digest, nil
		}
	}
	return "", fmt.Errorf("resolve %s: index carries no linux/%s manifest",
		reference, reference.Architecture)
}

// ListTags returns every tag a repository carries, following pagination. A
// repository with many tags answers in pages, and stopping at the first page
// would compare against whichever tags happened to land there.
func (c *Client) ListTags(reference Reference) ([]string, error) {
	path := "/v2/" + reference.Repository + "/tags/list"
	var tags []string
	for range 50 {
		response, err := c.do(reference.Registry, path, "application/json")
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return nil, fmt.Errorf("list tags for %s: %s", reference.Repository, response.Status)
		}
		var body struct {
			Tags []string `json:"tags"`
		}
		err = json.NewDecoder(response.Body).Decode(&body)
		next := nextPage(response.Header.Get("Link"))
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode tags for %s: %w", reference.Repository, err)
		}
		tags = append(tags, body.Tags...)
		if next == "" {
			return tags, nil
		}
		path = next
	}
	return tags, nil
}

// nextPage reads the rel=next target out of a Link header.
func nextPage(link string) string {
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end <= start {
			continue
		}
		return part[start+1 : end]
	}
	return ""
}
