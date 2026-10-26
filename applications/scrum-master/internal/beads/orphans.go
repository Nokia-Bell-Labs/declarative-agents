// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// localOrigin is the clone's origin when it is a local path, else empty:
// an origin that is a URL is not judged, since telling whether it still
// resolves needs the network.
func localOrigin(clone string) string {
	b, err := exec.Command("git", "-C", clone, "config", "--get", "remote.origin.url").Output()
	origin := strings.TrimPrefix(strings.TrimSpace(string(b)), "file://")
	if err != nil || !filepath.IsAbs(origin) {
		return ""
	}
	return origin
}

// gone reports whether the clone's local origin no longer exists (GH-116).
func gone(clone string) bool {
	origin := localOrigin(clone)
	if origin == "" {
		return false
	}
	_, err := os.Stat(origin)
	return errors.Is(err, fs.ErrNotExist)
}

// remade reports whether the clone's local origin exists but was made again
// since the clone fetched from it, holding none of that history, as a
// scratch program deleted and created again at the same path is (GH-141).
func remade(clone string) bool {
	origin := localOrigin(clone)
	if origin == "" || gone(clone) {
		return false
	}
	out, _ := exec.Command("git", "-C", clone, "for-each-ref", "--format=%(objectname)", "refs/remotes/origin").Output()
	fetched := strings.Fields(string(out))
	if len(fetched) == 0 {
		return false
	}
	for _, sha := range fetched {
		if exec.Command("git", "-C", origin, "cat-file", "-e", sha+"^{commit}").Run() == nil {
			return false
		}
	}
	return true
}

// OrphanedClones lists each tracker clone under the user cache that follows
// no origin, with why: its local origin is gone (GH-116), or was made again
// holding none of its history, which OpenRepository sets aside (GH-141).
// Each is named, never removed, since it may hold the only copy of a
// tracker's commits; the person copies what they need and removes it.
func OrphanedClones() []string {
	root, err := cachedClones()
	if err != nil {
		return nil
	}
	gits, _ := filepath.Glob(filepath.Join(root, "*", ".git"))
	var out []string
	for _, g := range gits {
		clone := filepath.Dir(g)
		switch {
		case gone(clone):
			out = append(out, clone+" (origin "+localOrigin(clone)+" is gone)")
		case remade(clone):
			out = append(out, clone+" (origin "+localOrigin(clone)+" was made again and holds none of its history)")
		}
	}
	return out
}
