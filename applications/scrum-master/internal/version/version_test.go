// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package version

import "testing"

// A stamped release wins over the build info, so a published scrum-master names its
// own release whatever its module version reads (GH-18).
func TestCurrentPrefersTheStampedRelease(t *testing.T) {
	saved := Release
	defer func() { Release = saved }()
	Release = "v0.20990101.3"
	if got := Current(); got != "v0.20990101.3" {
		t.Fatalf("Current = %q, want the stamped release", got)
	}
	Release = ""
	if got := Current(); got == "v0.20990101.3" {
		t.Fatalf("Current = %q after the stamp was cleared", got)
	}
}

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{"v0.20261022.2": true, "dev": false, "v1.2.3": false, "v0.2026102.1": false} {
		if IsRelease(v) != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, !want, want)
		}
	}
}
