// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pinsurvey

import (
	"errors"
	"strings"
	"testing"
)

const otherDigest = "sha256:" +
	"2222222222222222222222222222222222222222222222222222222222222222"

// fakeResolver answers from maps, so a verdict test states its registry in
// one literal instead of a server.
type fakeResolver struct {
	digests map[string]string
	tags    map[string][]string
	err     error
	tagErr  error
}

func (f fakeResolver) ResolveDigest(reference Reference) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	digest, ok := f.digests[reference.Repository+":"+reference.Tag]
	if !ok {
		return "", errors.New("manifest unknown")
	}
	return digest, nil
}

func (f fakeResolver) ListTags(reference Reference) ([]string, error) {
	if f.tagErr != nil {
		return nil, f.tagErr
	}
	return f.tags[reference.Repository], nil
}

func pin(image, tag, digest string) Pin {
	return Pin{Location: "magefiles/kindrig/demo.go traefikImageDigests", Image: image, Tag: tag, Digest: digest}
}

func only(t *testing.T, results []Result) Result {
	t.Helper()
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d: %v", len(results), results)
	}
	return results[0]
}

func TestMatchingDigestAndNewestTagIsCurrent(t *testing.T) {
	resolver := fakeResolver{
		digests: map[string]string{"library/traefik:v3.7.13": testDigest},
		tags:    map[string][]string{"library/traefik": {"v3.7.10", "v3.7.13"}},
	}
	result := only(t, Survey(resolver, []Pin{pin("traefik", "v3.7.13", testDigest)}))
	if result.Verdict != Current {
		t.Fatalf("verdict = %q, want current: %s", result.Verdict, result)
	}
}

// srd006 R2.1: a re-pushed tag outranks staleness.
func TestChangedDigestIsMoved(t *testing.T) {
	resolver := fakeResolver{
		digests: map[string]string{"library/traefik:v3.7.10": otherDigest},
		tags:    map[string][]string{"library/traefik": {"v3.7.10", "v3.7.13"}},
	}
	result := only(t, Survey(resolver, []Pin{pin("traefik", "v3.7.10", testDigest)}))
	if result.Verdict != Moved {
		t.Fatalf("verdict = %q, want moved", result.Verdict)
	}
	message := result.String()
	for _, want := range []string{testDigest, otherDigest, "traefik"} {
		if !strings.Contains(message, want) {
			t.Errorf("message %q does not name %q", message, want)
		}
	}
}

// srd006 R3.1.
func TestNewerReleaseIsBehind(t *testing.T) {
	resolver := fakeResolver{
		digests: map[string]string{"library/traefik:v3.7.10": testDigest},
		tags:    map[string][]string{"library/traefik": {"v3.7.9", "v3.7.10", "v3.7.13"}},
	}
	result := only(t, Survey(resolver, []Pin{pin("traefik", "v3.7.10", testDigest)}))
	if result.Verdict != Behind {
		t.Fatalf("verdict = %q, want behind", result.Verdict)
	}
	if result.Latest != "v3.7.13" {
		t.Fatalf("latest = %q, want v3.7.13", result.Latest)
	}
	if !strings.Contains(result.String(), "v3.7.10") || !strings.Contains(result.String(), "v3.7.13") {
		t.Errorf("message names only one version: %s", result)
	}
}

// srd006 R3.2: a tag that names no version has no successor to compare.
func TestNonVersionTagComparesOnIntegrityAlone(t *testing.T) {
	for _, tag := range []string{"must-be-overridden-with-git-revision", "a1b2c3d4e5f6", "latest"} {
		t.Run(tag, func(t *testing.T) {
			resolver := fakeResolver{
				digests: map[string]string{"declarative-agents/smoke:" + tag: testDigest},
				tags:    map[string][]string{"declarative-agents/smoke": {"9.9.9"}},
			}
			result := only(t, Survey(resolver, []Pin{pin("declarative-agents/smoke", tag, testDigest)}))
			if result.Verdict != Current {
				t.Fatalf("verdict = %q, want current", result.Verdict)
			}
		})
	}
}

// srd006 R3.2: a pre-release or variant tag is not a later release.
func TestPrereleaseAndVariantTagsDoNotCountAsNewer(t *testing.T) {
	resolver := fakeResolver{
		digests: map[string]string{"ollama/ollama:0.34.2": testDigest},
		tags: map[string][]string{"ollama/ollama": {
			"0.34.2", "0.34.3-rc1", "0.35.0-rc0", "0.34.2-rocm", "rocm", "latest"}},
	}
	result := only(t, Survey(resolver, []Pin{pin("ollama/ollama", "0.34.2", testDigest)}))
	if result.Verdict != Current {
		t.Fatalf("verdict = %q, want current; latest=%q", result.Verdict, result.Latest)
	}
}

func TestVPrefixAndBareSemverBothCompare(t *testing.T) {
	cases := map[string][]string{"v1.36.1": {"v1.36.1", "v1.37.0"}, "1.36.1": {"1.36.1", "1.37.0"}}
	for tag, tags := range cases {
		t.Run(tag, func(t *testing.T) {
			resolver := fakeResolver{
				digests: map[string]string{"kindest/node:" + tag: testDigest},
				tags:    map[string][]string{"kindest/node": tags},
			}
			result := only(t, Survey(resolver, []Pin{pin("kindest/node", tag, testDigest)}))
			if result.Verdict != Behind {
				t.Fatalf("verdict = %q, want behind", result.Verdict)
			}
		})
	}
}

// srd006 R5.1: an unreachable registry is never current.
func TestUnreachableRegistryIsUnknown(t *testing.T) {
	resolver := fakeResolver{err: errors.New("dial tcp: no route to host")}
	result := only(t, Survey(resolver, []Pin{pin("traefik", "v3.7.10", testDigest)}))
	if result.Verdict != Unknown {
		t.Fatalf("verdict = %q, want unknown", result.Verdict)
	}
	if !strings.Contains(result.String(), "no route to host") {
		t.Errorf("message does not name the reason: %s", result)
	}
}

func TestAbsentTagIsUnknown(t *testing.T) {
	resolver := fakeResolver{digests: map[string]string{}}
	result := only(t, Survey(resolver, []Pin{pin("traefik", "v9.9.9", testDigest)}))
	if result.Verdict != Unknown {
		t.Fatalf("verdict = %q, want unknown", result.Verdict)
	}
}

// A tag list that cannot be read leaves currency unanswered, so the pin is
// unknown rather than current on the strength of its digest alone.
func TestUnreadableTagListIsUnknown(t *testing.T) {
	resolver := fakeResolver{
		digests: map[string]string{"library/traefik:v3.7.10": testDigest},
		tagErr:  errors.New("tags/list: 500 Internal Server Error"),
	}
	result := only(t, Survey(resolver, []Pin{pin("traefik", "v3.7.10", testDigest)}))
	if result.Verdict != Unknown {
		t.Fatalf("verdict = %q, want unknown", result.Verdict)
	}
}

// srd006 R2.1: moved sorts ahead of everything, and unknown ahead of behind,
// so the report opens with what needs a decision.
func TestReportOrdersMovedFirstThenUnknownThenBehind(t *testing.T) {
	resolver := fakeResolver{
		digests: map[string]string{
			"library/moved:1.0.0":   otherDigest,
			"library/behind:1.0.0":  testDigest,
			"library/current:2.0.0": testDigest,
		},
		tags: map[string][]string{
			"library/behind":  {"1.0.0", "2.0.0"},
			"library/current": {"2.0.0"},
		},
	}
	pins := []Pin{
		{Location: "d", Image: "current", Tag: "2.0.0", Digest: testDigest},
		{Location: "c", Image: "behind", Tag: "1.0.0", Digest: testDigest},
		{Location: "b", Image: "absent", Tag: "1.0.0", Digest: testDigest},
		{Location: "a", Image: "moved", Tag: "1.0.0", Digest: testDigest},
	}
	got := []Verdict{}
	for _, result := range Survey(resolver, pins) {
		got = append(got, result.Verdict)
	}
	want := []Verdict{Moved, Unknown, Behind, Current}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// A pin with no recorded digest is checked for currency only: the kind node
// image records one, the smoke registry entries do not.
func TestPinWithoutADigestIsStillCheckedForCurrency(t *testing.T) {
	resolver := fakeResolver{
		digests: map[string]string{"library/chroma:1.5.3": testDigest},
		tags:    map[string][]string{"library/chroma": {"1.5.3", "1.6.0"}},
	}
	result := only(t, Survey(resolver, []Pin{{Location: "x", Image: "chroma", Tag: "1.5.3"}}))
	if result.Verdict != Behind {
		t.Fatalf("verdict = %q, want behind", result.Verdict)
	}
}

func TestCountsSummarizesEachVerdict(t *testing.T) {
	counts := Counts([]Result{{Verdict: Moved}, {Verdict: Behind}, {Verdict: Behind}, {Verdict: Current}})
	if counts[Behind] != 2 || counts[Moved] != 1 || counts[Current] != 1 || counts[Unknown] != 0 {
		t.Fatalf("counts = %v", counts)
	}
}

// A repository often publishes both 3.7.13 and v3.7.13. The report names the
// one written the way the pin is, so the reported tag is the one to paste.
func TestReportedTagMatchesThePinnedStyle(t *testing.T) {
	cases := map[string]string{"v3.7.10": "v3.7.13", "3.7.10": "3.7.13"}
	for pinned, want := range cases {
		t.Run(pinned, func(t *testing.T) {
			resolver := fakeResolver{
				digests: map[string]string{"library/traefik:" + pinned: testDigest},
				tags:    map[string][]string{"library/traefik": {"3.7.13", "v3.7.13", pinned}},
			}
			result := only(t, Survey(resolver, []Pin{pin("traefik", pinned, testDigest)}))
			if result.Latest != want {
				t.Fatalf("latest = %q, want %q", result.Latest, want)
			}
		})
	}
}

// The same image reached by two spellings is one pin, not two.
func TestDeduplicateCollapsesRegistrySpellings(t *testing.T) {
	pins := []Pin{
		{Location: "applications/chatbot-mesh/helm [defaults]", Image: "alpine/k8s", Tag: "1.31.4"},
		{Location: "magefiles/kindrig/clidonor.go", Image: "docker.io/alpine/k8s", Tag: "1.31.4"},
	}
	if unique := Deduplicate(pins); len(unique) != 1 {
		t.Fatalf("deduplicate kept %d pins: %v", len(unique), unique)
	}
}
