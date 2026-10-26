// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package goals

import (
	"os/exec"
	"testing"
)

// With SCRUM_MASTER_WEB_BASE unset, the web base is the root of the
// checkout's origin as configured, whatever url.insteadOf rewrites it to;
// the variable, when set, wins (GH-109).
func TestWebBaseFromOrigin(t *testing.T) {
	for origin, want := range map[string]string{
		"https://git.example.com/acme/system.git":    "https://git.example.com",
		"https://token@git.example.com/acme/system":  "https://git.example.com",
		"git@git.example.com:acme/system.git":        "https://git.example.com",
		"ssh://git@git.example.com:2222/acme/system": "https://git.example.com",
	} {
		dir := t.TempDir()
		for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", origin},
			{"config", "url./elsewhere/.insteadOf", "https://git.example.com/"}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
		t.Setenv("SCRUM_MASTER_WEB_BASE", "")
		if got, err := WebBase(dir); err != nil || got != want {
			t.Errorf("origin %s: WebBase = %q, %v; want %q", origin, got, err, want)
		}
		t.Setenv("SCRUM_MASTER_WEB_BASE", "https://cite.example.com/")
		if got, _ := WebBase(dir); got != "https://cite.example.com" {
			t.Errorf("origin %s: the variable lost to the origin: %q", origin, got)
		}
	}
	t.Setenv("SCRUM_MASTER_WEB_BASE", "")
	if _, err := WebBase(t.TempDir()); err == nil {
		t.Error("no origin and no variable gave a web base")
	}
}
