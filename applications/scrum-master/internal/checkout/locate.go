// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package checkout

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// ErrNoCheckout is a project with no checkout to read: none of the folders
// searched holds a checkout whose origin is that project (GH-2744).
var ErrNoCheckout = errors.New("no checkout of the project")

// ProjectPath reduces a repository URL or path to GROUP/REPO: an https or
// ssh URL, or the scp form user@host:group/repo that git's url.insteadOf
// often rewrites an origin to (GH-67).
func ProjectPath(repo string) string {
	repo = strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
	if _, rest, ok := strings.Cut(repo, "://"); ok {
		_, repo, _ = strings.Cut(rest, "/")
	} else if host, p, ok := strings.Cut(repo, ":"); ok && !strings.Contains(host, "/") && p != "" {
		repo = p
	}
	if p, _, ok := strings.Cut(repo, "/-/"); ok {
		repo = p
	}
	return strings.Trim(repo, "/")
}

// Origin is the checkout's configured origin, as written, not as
// url.insteadOf expands it; empty when it has none.
func Origin(dir string) string {
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// toplevel is the root of the checkout holding dir, dir when git cannot
// say.
func toplevel(dir string) string {
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return dir
}

// isCheckout reports whether dir is a git checkout's root.
func isCheckout(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Of reports whether the checkout at dir is project's: its origin names the
// project, or, for an origin that is a local path or none, its repository
// is named as the project's last segment.
func Of(dir, project string) bool {
	if !isCheckout(dir) {
		return false
	}
	origin := Origin(dir)
	if origin != "" && !strings.HasPrefix(origin, "/") && !strings.HasPrefix(origin, ".") {
		return ProjectPath(origin) == ProjectPath(project)
	}
	name := filepath.Base(mainOf(dir))
	if origin != "" {
		name = strings.TrimSuffix(filepath.Base(origin), ".git")
	}
	return name == path.Base(project)
}

// MainCheckout is the main checkout of dir's repository, dir itself for one
// or when git cannot say: a worktree, which pop names for its branch, is
// named by the repository it is of (GH-2752).
func MainCheckout(dir string) string { return mainOf(dir) }

// IsForge reports whether an origin names a forge project, not a local
// path.
func IsForge(origin string) bool {
	return origin != "" && !strings.HasPrefix(origin, "/") && !strings.HasPrefix(origin, ".")
}

// mainOf is the main checkout of dir's repository, dir itself for one or
// when git cannot say: a worktree is named by the repository it is of.
func mainOf(dir string) string {
	common := commonDir(dir)
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	return dir
}

// pathCheckout is the checkout a filesystem path names: an absolute path,
// or one relative to dir, that holds a git checkout; never a project path.
func pathCheckout(dir, project string) (string, bool) {
	if !filepath.IsAbs(project) && !strings.HasPrefix(project, ".") {
		return "", false
	}
	p := project
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	root := toplevel(p)
	if !isCheckout(root) {
		return "", false
	}
	return root, true
}

// Levels are the folders whose checkouts are searched from the checkout
// at dir, nearest first: the folder holding it, then the folder holding
// each folder above it, up to, and not including, the user's home folder,
// as checkouts that mirror a forge's group layout sit (GH-2743). A
// checkout outside the home folder searches the folder holding it alone.
func Levels(dir string) []string {
	root := toplevel(dir)
	levels := []string{filepath.Dir(root)}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return levels
	}
	if h, err := filepath.EvalSymlinks(home); err == nil {
		home = h
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	rel, err := filepath.Rel(home, root)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return levels
	}
	levels = levels[:0]
	for cur := filepath.Dir(root); cur != home && strings.HasPrefix(cur, home+string(filepath.Separator)); cur = filepath.Dir(cur) {
		levels = append(levels, cur)
	}
	return levels
}

// Locate is project's checkout as seen from the checkout at dir: dir's own
// when it is the project's, else the nearest folder of Levels holding a
// checkout named for the project whose origin is the project. A checkout
// named for the project whose origin is another is never taken for it
// (GH-2744).
func Locate(dir, project string) (string, bool) {
	// A project named by a checkout's path is that checkout, whatever its
	// origin: words document --system as the system repository's path
	// (GH-2769).
	if p, ok := pathCheckout(dir, project); ok {
		return p, true
	}
	root := toplevel(dir)
	if Of(root, project) {
		return root, true
	}
	for _, level := range Levels(root) {
		if c := filepath.Join(level, path.Base(project)); c != root && Of(c, project) {
			return c, true
		}
	}
	return "", false
}
