// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

func TestOllamaCacheIdentityKeysImageAndCanonicalModelSet(t *testing.T) {
	image := "sha256:" + strings.Repeat("a", 64)
	first, err := ollamaCacheIdentity(image, []string{"chat", "embed", "chat"})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := ollamaCacheIdentity(image, []string{"embed", "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if first != reordered {
		t.Fatalf("canonical model set identities differ: %s != %s", first, reordered)
	}
	changedModel, _ := ollamaCacheIdentity(image, []string{"embed", "other"})
	changedImage, _ := ollamaCacheIdentity(
		"sha256:"+strings.Repeat("b", 64), []string{"embed", "chat"})
	if first == changedModel || first == changedImage {
		t.Fatal("cache identity did not change with model or image digest")
	}
}

func TestAggregateOllamaCacheReusesOnlyReadyMatchingIdentity(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	var calls []string
	session.kindRun = func(args ...string) ([]byte, error) {
		calls = append(calls, "kind "+strings.Join(args, " "))
		return nil, nil
	}
	session.cluster = kindrig.Cluster{Name: kindrig.PlatformClusterName, Created: true}
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()

	image := "sha256:" + strings.Repeat("a", 64)
	identity, err := ollamaCacheIdentity(image, helmLLMModels)
	if err != nil {
		t.Fatal(err)
	}
	run := func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if strings.Contains(call, `test -f "$1/ready"`) {
			return []byte(identity + "\n"), nil
		}
		return nil, nil
	}
	cache, err := prepareAggregateOllamaCache(
		run, session.cluster, image, helmLLMModels)
	if err != nil {
		t.Fatal(err)
	}
	if !cache.Reused || cache.Identity != identity ||
		!strings.HasSuffix(cache.HostPath, identity) {
		t.Fatalf("cache=%+v, want matching ready reuse", cache)
	}
	for _, call := range calls {
		if strings.Contains(call, `rm -rf "$1" &&`) {
			t.Fatalf("matching cache was reset: %s", call)
		}
	}
	if !strings.Contains(strings.Join(calls, "\n"), `rm -rf "$1/active"`) {
		t.Fatalf("matching cache did not reset scenario storage: %v", calls)
	}
	session.close()
	// The cache lives with the platform node, so closing the session keeps it
	// warm for the next run and only an owned platform's deletion removes it.
	joined := strings.Join(calls, "\n")
	if strings.Contains(joined, "rm -rf "+aggregateOllamaCacheRoot) {
		t.Fatalf("session close removed the platform model cache:\n%s", joined)
	}
}

func TestAggregateOllamaCacheResetsStaleOrIncompleteState(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	session.cluster = kindrig.Cluster{Name: kindrig.PlatformClusterName, Created: true}
	session.kindRun = func(...string) ([]byte, error) { return nil, nil }
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if strings.Contains(call, `test -f "$1/ready"`) {
			return []byte("stale"), errors.New("not ready")
		}
		return nil, nil
	}
	image := "sha256:" + strings.Repeat("c", 64)
	cache, err := prepareAggregateOllamaCache(
		run, session.cluster, image, helmLLMModels)
	if err != nil {
		t.Fatal(err)
	}
	if cache.Reused {
		t.Fatal("stale cache was reused")
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, `rm -rf "$1"`) {
		t.Fatalf("stale cache was not reset: %v", calls)
	}
	if !strings.Contains(joined, `! -path "$1" -exec rm -rf`) {
		t.Fatalf("stale cache identities were not pruned: %v", calls)
	}
}

// A platform the scenario does not own (a release run's or platform:up's) still
// hosts the cache: that is what keeps it warm across runs on a persistent
// platform (GH-2215).
func TestOllamaCacheUsesUnownedPlatformNode(t *testing.T) {
	var calls []string
	cache, err := prepareAggregateOllamaCache(
		func(name string, args ...string) ([]byte, error) {
			call := name + " " + strings.Join(args, " ")
			calls = append(calls, call)
			if strings.Contains(call, `test -f "$1/ready"`) {
				return nil, errors.New("no cache yet")
			}
			return nil, nil
		},
		kindrig.Cluster{Name: kindrig.PlatformClusterName},
		"sha256:"+strings.Repeat("d", 64),
		helmLLMModels,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cache.HostPath == "" || len(calls) == 0 ||
		!strings.Contains(calls[0], "docker exec "+kindrig.PlatformClusterName+"-control-plane") {
		t.Fatalf("cache=%+v calls=%v, want the platform node cache prepared", cache, calls)
	}
}
