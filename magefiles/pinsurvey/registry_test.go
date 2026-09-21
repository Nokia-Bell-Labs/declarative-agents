// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pinsurvey

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testDigest = "sha256:" +
	"1111111111111111111111111111111111111111111111111111111111111111"

// fakeRegistry serves the distribution endpoints the survey calls. Every test
// drives this rather than a network, so the suite proves the client's
// behaviour and never an upstream project's release cadence.
type fakeRegistry struct {
	digests     map[string]string
	tags        map[string][]string
	pages       map[string][]string
	requireAuth bool
	tokenIssued int
	authorized  []bool
}

func (s *fakeRegistry) start(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		s.tokenIssued++
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "issued"})
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		if s.requireAuth {
			s.authorized = append(s.authorized, r.Header.Get("Authorization") == "Bearer issued")
			if r.Header.Get("Authorization") != "Bearer issued" {
				w.Header().Set("WWW-Authenticate",
					`Bearer realm="https://auth.example/token",service="registry",scope="repository:x:pull"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		if page, ok := s.pages[r.URL.RequestURI()]; ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"tags": page})
			return
		}
		if tags, ok := s.tags[r.URL.Path]; ok {
			if next, ok := s.pages["next:"+r.URL.Path]; ok && len(next) > 0 {
				w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next[0]))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tags": tags})
			return
		}
		if digest, ok := s.digests[r.URL.Path]; ok {
			w.Header().Set("Docker-Content-Digest", digest)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client := NewClient()
	client.BaseURL = server.URL
	return client
}

func TestParseReferenceSplitsRegistryRepositoryTagAndDigest(t *testing.T) {
	cases := map[string]Reference{
		"busybox:1.36": {
			Registry: defaultRegistry, Repository: "library/busybox", Tag: "1.36"},
		"ollama/ollama:0.34.2": {
			Registry: defaultRegistry, Repository: "ollama/ollama", Tag: "0.34.2"},
		"ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0": {
			Registry: "ghcr.io", Repository: "nokia-bell-labs/declarative-agents/agent-core", Tag: "0.1.0"},
		"registry.k8s.io/metrics-server/metrics-server:v0.7.2": {
			Registry: "registry.k8s.io", Repository: "metrics-server/metrics-server", Tag: "v0.7.2"},
		"localhost:5000/agent-core:1.2.3": {
			Registry: "localhost:5000", Repository: "agent-core", Tag: "1.2.3"},
		"docker.io/alpine/k8s:1.31.4@" + testDigest: {
			Registry: defaultRegistry, Repository: "alpine/k8s", Tag: "1.31.4", Digest: testDigest},
		"index.docker.io/library/traefik:v3.7.10": {
			Registry: defaultRegistry, Repository: "library/traefik", Tag: "v3.7.10"},
	}
	for image, want := range cases {
		t.Run(image, func(t *testing.T) {
			got, err := ParseReference(image)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestParseReferenceRejectsEmpty(t *testing.T) {
	if _, err := ParseReference("   "); err == nil {
		t.Fatal("accepted an empty reference")
	}
}

func TestResolveDigestReadsTheContentDigest(t *testing.T) {
	registry := &fakeRegistry{digests: map[string]string{
		"/v2/library/busybox/manifests/1.36": testDigest,
	}}
	client := registry.start(t)
	reference, _ := ParseReference("busybox:1.36")
	digest, err := client.ResolveDigest(reference)
	if err != nil {
		t.Fatal(err)
	}
	if digest != testDigest {
		t.Fatalf("digest = %q, want %q", digest, testDigest)
	}
}

// A registry that challenges is answered once and the request retried. This
// is the flow docker.io, ghcr.io and registry.k8s.io all use.
func TestChallengeThenTokenThenRetry(t *testing.T) {
	registry := &fakeRegistry{
		requireAuth: true,
		digests:     map[string]string{"/v2/library/busybox/manifests/1.36": testDigest},
	}
	client := registry.start(t)
	reference, _ := ParseReference("busybox:1.36")
	digest, err := client.ResolveDigest(reference)
	if err != nil {
		t.Fatal(err)
	}
	if digest != testDigest {
		t.Fatalf("digest = %q", digest)
	}
	if registry.tokenIssued != 1 {
		t.Errorf("token issued %d times, want 1", registry.tokenIssued)
	}
	if len(registry.authorized) != 2 || registry.authorized[0] || !registry.authorized[1] {
		t.Errorf("authorization sequence = %v, want [false true]", registry.authorized)
	}
	// A second call reuses the token rather than challenging again.
	if _, err := client.ResolveDigest(reference); err != nil {
		t.Fatal(err)
	}
	if registry.tokenIssued != 1 {
		t.Errorf("token issued %d times across two calls, want 1", registry.tokenIssued)
	}
}

func TestResolveDigestReportsAnAbsentTag(t *testing.T) {
	client := (&fakeRegistry{}).start(t)
	reference, _ := ParseReference("library/nothing:9.9.9")
	if _, err := client.ResolveDigest(reference); err == nil {
		t.Fatal("absent tag resolved without error")
	}
}

func TestListTagsFollowsPagination(t *testing.T) {
	registry := &fakeRegistry{
		tags: map[string][]string{"/v2/library/traefik/tags/list": {"v3.7.10"}},
		pages: map[string][]string{
			"next:/v2/library/traefik/tags/list":         {"/v2/library/traefik/tags/list?last=v3.7.10"},
			"/v2/library/traefik/tags/list?last=v3.7.10": {"v3.7.13"},
		},
	}
	client := registry.start(t)
	reference, _ := ParseReference("traefik:v3.7.10")
	tags, err := client.ListTags(reference)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "v3.7.10" || tags[1] != "v3.7.13" {
		t.Fatalf("tags = %v, want both pages", tags)
	}
}

func TestNextPageReadsTheLinkHeader(t *testing.T) {
	cases := map[string]string{
		`</v2/x/tags/list?last=a>; rel="next"`:                      "/v2/x/tags/list?last=a",
		`</v2/x/tags/list?last=a>; rel="prev", </v2/y>; rel="next"`: "/v2/y",
		``:                    "",
		`</v2/x>; rel="prev"`: "",
		`malformed`:           "",
	}
	for header, want := range cases {
		if got := nextPage(header); got != want {
			t.Errorf("nextPage(%q) = %q, want %q", header, got, want)
		}
	}
}

// The rig records per-architecture digests, so a reference naming an
// architecture must read that platform's entry out of the index rather than
// the index digest the header carries (srd006 R1.2).
func TestResolveDigestReadsOnePlatformOutOfAnIndex(t *testing.T) {
	index := `{"manifests":[
	  {"digest":"` + testDigest + `","platform":{"os":"linux","architecture":"amd64"}},
	  {"digest":"` + otherDigest + `","platform":{"os":"linux","architecture":"arm64"}}]}`
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/library/traefik/manifests/v3.7.10", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Docker-Content-Digest", "sha256:"+
			"3333333333333333333333333333333333333333333333333333333333333333")
		_, _ = w.Write([]byte(index))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client := NewClient()
	client.BaseURL = server.URL

	reference, _ := ParseReference("traefik:v3.7.10")
	for architecture, want := range map[string]string{"amd64": testDigest, "arm64": otherDigest} {
		reference.Architecture = architecture
		got, err := client.ResolveDigest(reference)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s digest = %q, want %q", architecture, got, want)
		}
	}
	reference.Architecture = "riscv64"
	if _, err := client.ResolveDigest(reference); err == nil {
		t.Error("an absent platform resolved without error")
	}
	reference.Architecture = ""
	got, err := client.ResolveDigest(reference)
	if err != nil || !strings.HasPrefix(got, "sha256:3333") {
		t.Errorf("index digest = %q, %v", got, err)
	}
}

// docker.io names the website, not the distribution API. A reference written
// that way must reach registry-1.docker.io, or every Hub pin spelled with an
// explicit registry reports unknown.
func TestHubAliasesResolveToTheAPIHost(t *testing.T) {
	for _, alias := range []string{"docker.io", "index.docker.io"} {
		reference, err := ParseReference(alias + "/alpine/k8s:1.31.4")
		if err != nil {
			t.Fatal(err)
		}
		if reference.Registry != defaultRegistry {
			t.Errorf("%s resolved to %q, want %q", alias, reference.Registry, defaultRegistry)
		}
	}
	reference, _ := ParseReference("ghcr.io/owner/image:1.0.0")
	if reference.Registry != "ghcr.io" {
		t.Errorf("ghcr.io was rewritten to %q", reference.Registry)
	}
}
