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

// Releases order by date, then by their number that day, the number read as
// a number; what is no release orders against nothing (GH-2899).
func TestOlder(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v0.20261103.2", "v0.20261103.4", true},
		{"v0.20261103.4", "v0.20261103.2", false},
		{"v0.20261103.4", "v0.20261103.4", false},
		{"v0.20261103.9", "v0.20261103.10", true},
		{"v0.20261102.9", "v0.20261103.1", true},
		{"v0.20261201.1", "v0.20261103.9", false},
		{"dev", "v0.20261103.4", false},
		{"v0.20261103.4", "dev", false},
		{"source:/tree", "v0.20261103.4", false},
		{"v0.20261103.4", "", false},
		{"v9.9.9", "v0.20261103.4", false},
	} {
		if got := Older(c.a, c.b); got != c.want {
			t.Errorf("Older(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
