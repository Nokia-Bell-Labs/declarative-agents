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

func TestSiblingLocatesThisRepository(t *testing.T) {
	rel, ok, err := Sibling(".", nil)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
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
