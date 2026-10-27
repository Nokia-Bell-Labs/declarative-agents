// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
)

const tag = "v0.20260101.1"

func TestDefaultPinIsAReleaseTag(t *testing.T) {
	if pin := DefaultPin(); !version.IsRelease(pin) {
		t.Fatalf("DefaultPin %q is not a release tag", pin)
	}
}

// The runtime resolves for this application wherever it sits: the sibling
// agent-core inside the repository that ships it, else the parent checkout
// or release it names (GH-2754).
func TestTheRuntimeResolvesForThisApplication(t *testing.T) {
	rel, err := Resolve(filepath.Join("..", ".."), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, serr := os.Stat(filepath.Join(rel.CoreRoot(), "cmd", "agent")); serr != nil {
		t.Fatalf("core root %q has no agent: %v", rel.CoreRoot(), serr)
	}
	if _, ok, _ := Sibling(t.TempDir(), nil); ok {
		t.Fatal("a directory outside the repository must not be a sibling")
	}
}

func TestDefaultPinFollowsTheStampedRelease(t *testing.T) {
	saved := version.Release
	defer func() { version.Release = saved }()
	version.Release = "v0.20990101.4"
	if got := DefaultPin(); got != "v0.20990101.4" {
		t.Fatalf("DefaultPin = %q, want the stamped release", got)
	}
}

// A binary built against a pinned runtime release pins that release in a
// new corpus, not its own version (GH-2755).
func TestDefaultPinIsTheRuntimeBuiltAgainst(t *testing.T) {
	savedRelease, savedRuntime := version.Release, version.Runtime
	defer func() { version.Release, version.Runtime = savedRelease, savedRuntime }()
	version.Release, version.Runtime = "v0.20990101.4", "v0.20261027.0"
	if got := DefaultPin(); got != "v0.20261027.0" {
		t.Fatalf("DefaultPin = %q, want the runtime release built against", got)
	}
}
