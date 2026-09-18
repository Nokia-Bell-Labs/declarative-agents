// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

type fakeReleasePlatform struct {
	mu      sync.Mutex
	started int
	stopped []bool
	startFn func() error
}

func (f *fakeReleasePlatform) platform() *releasePlatform {
	return &releasePlatform{
		start: func(kindrig.PlatformOptions) (*kindrig.Platform, error) {
			f.mu.Lock()
			f.started++
			f.mu.Unlock()
			if f.startFn != nil {
				if err := f.startFn(); err != nil {
					return nil, err
				}
			}
			return &kindrig.Platform{Cluster: kindrig.Cluster{Name: kindrig.PlatformClusterName}}, nil
		},
		stopFn: func(_ *kindrig.Platform, failed bool) {
			f.mu.Lock()
			f.stopped = append(f.stopped, failed)
			f.mu.Unlock()
		},
	}
}

// runRecording runs in-process gates through their own run func, the way
// runReleaseCommand does, and records the launch order.
func runRecording(order *[]string, mu *sync.Mutex, fail map[string]error) releaseCommandRunner {
	return func(gate releaseGate) error {
		mu.Lock()
		*order = append(*order, gate.name)
		mu.Unlock()
		if gate.run != nil {
			return gate.run()
		}
		return fail[gate.name]
	}
}

func TestPlatformGateFollowsRootAuditAndPrecedesApplicationGates(t *testing.T) {
	gates := withPlatformGate(releaseGates("/release"), (&releasePlatform{}).gate())
	if gates[0].name != "root audit" || gates[1].name != releasePlatformGateName ||
		!gates[1].exclusive || gates[1].run == nil {
		t.Fatalf("leading gates = %q, %q (exclusive=%v)", gates[0].name, gates[1].name, gates[1].exclusive)
	}
	if len(gates) != len(releaseGates("/release"))+1 {
		t.Fatalf("gate count = %d, want release gates plus the platform", len(gates))
	}
}

func TestReleasePlatformBootsBeforeApplicationsAndTearsDownAfterSuccess(t *testing.T) {
	fake := &fakeReleasePlatform{}
	var order []string
	var mu sync.Mutex
	err := executePlatformReleaseGates(releaseGates("/release"), fake.platform(),
		runRecording(&order, &mu, nil))
	if err != nil {
		t.Fatal(err)
	}
	if order[0] != "root audit" || order[1] != releasePlatformGateName {
		t.Fatalf("order = %v, want root audit then platform", order)
	}
	if fake.started != 1 || len(fake.stopped) != 1 || fake.stopped[0] {
		t.Fatalf("started=%d stopped=%v, want one boot and one clean teardown", fake.started, fake.stopped)
	}
}

func TestReleasePlatformTearsDownWithEvidenceWhenAnApplicationGateFails(t *testing.T) {
	fake := &fakeReleasePlatform{}
	gateErr := errors.New("helm smoke failed")
	var order []string
	var mu sync.Mutex
	err := executePlatformReleaseGates(releaseGates("/release"), fake.platform(),
		runRecording(&order, &mu, map[string]error{"applications/coding-agent integration": gateErr}))
	if !errors.Is(err, gateErr) {
		t.Fatalf("release error = %v, want %v", err, gateErr)
	}
	if len(fake.stopped) != 1 || !fake.stopped[0] {
		t.Fatalf("stopped = %v, want one failed teardown", fake.stopped)
	}
}

func TestReleasePlatformFailureBlocksApplicationGates(t *testing.T) {
	bootErr := errors.New("conformance: storage provisioner")
	fake := &fakeReleasePlatform{startFn: func() error { return bootErr }}
	var order []string
	var mu sync.Mutex
	err := executePlatformReleaseGates(releaseGates("/release"), fake.platform(),
		runRecording(&order, &mu, nil))
	if !errors.Is(err, bootErr) || !strings.Contains(err.Error(), "platform failed") {
		t.Fatalf("release error = %v, want platform gate failure", err)
	}
	if strings.Join(order, ",") != "root audit,platform" {
		t.Fatalf("order = %v, want nothing after the failed platform gate", order)
	}
	if len(fake.stopped) != 0 {
		t.Fatalf("stopped = %v; a platform that never booted has nothing to tear down", fake.stopped)
	}
}

func TestReleaseAuditFailureNeverBootsPlatform(t *testing.T) {
	fake := &fakeReleasePlatform{}
	auditErr := errors.New("audit failed")
	var order []string
	var mu sync.Mutex
	err := executePlatformReleaseGates(releaseGates("/release"), fake.platform(),
		runRecording(&order, &mu, map[string]error{"root audit": auditErr}))
	if !errors.Is(err, auditErr) || fake.started != 0 {
		t.Fatalf("err=%v started=%d, want audit failure without a platform", err, fake.started)
	}
}

func TestRunReleaseCommandRunsInProcessGate(t *testing.T) {
	called := false
	if err := runReleaseCommand(releaseGate{run: func() error { called = true; return nil }}); err != nil || !called {
		t.Fatalf("err=%v called=%v, want the in-process gate to run", err, called)
	}
}

func TestReleasePlatformEvidenceIsIgnoredBuildOutput(t *testing.T) {
	platform := newReleasePlatform("/release", "0123456789abcdef0123")
	want := "/release/build/kind-evidence/da-platform-0123456789ab"
	if platform.options.EvidenceDirectory != want {
		t.Fatalf("evidence directory = %q, want %q", platform.options.EvidenceDirectory, want)
	}
}
