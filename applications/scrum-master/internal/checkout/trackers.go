// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package checkout

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
)

// TrackersFile is where a program's tracking systems are configured, in
// its system repository (srd042 R1.3); the checkout holding it is the
// system repository's.
const TrackersFile = "control/trackers.md"

// FindTrackers is the trackers document for the checkout at dir: the checkout's
// own control/trackers.md, else the one sibling checkout that holds one, as
// repositories sit side by side under their owner and only the system
// repository has control files. Worktrees of one repository, which pop
// opens beside its checkout, are one sibling, read from the main checkout.
// It returns "" when there is none, and refuses siblings of more than one
// repository. Where checkouts mirror the forge's group layout, it also
// searches beside each folder above the checkout, nearest first, up to the
// user's home folder (Levels), taking there only a system checkout whose
// group holds this checkout's repository, so it never reaches another
// program's (GH-2743).
func FindTrackers(dir string) (string, error) {
	root := toplevel(dir)
	if own := filepath.Join(root, TrackersFile); exists(own) {
		return own, nil
	}
	ours := ""
	if o := Origin(root); o != "" && !strings.HasPrefix(o, "/") && !strings.HasPrefix(o, ".") {
		ours = ProjectPath(o)
	}
	for i, level := range Levels(root) {
		found, _ := filepath.Glob(filepath.Join(level, "*", TrackersFile))
		if i > 0 {
			if ours == "" {
				break
			}
			found = slicesFilter(found, func(f string) bool {
				o := Origin(filepath.Dir(filepath.Dir(f)))
				group := path.Dir(ProjectPath(o))
				return o != "" && group != "." && strings.HasPrefix(ours, group+"/")
			})
		}
		found = oneEachRepository(found, func(f string) string { return filepath.Dir(filepath.Dir(f)) })
		switch len(found) {
		case 0:
			continue
		case 1:
			return found[0], nil
		}
		where := "several sibling checkouts"
		if i > 0 {
			where = "several checkouts in " + level
		}
		return "", fmt.Errorf("%s hold %s (%s); run scrum-master from the system repository's checkout", where, TrackersFile, strings.Join(found, ", "))
	}
	return "", nil
}

func slicesFilter(xs []string, keep func(string) bool) []string {
	var out []string
	for _, x := range xs {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func globSiblings(root string) []string {
	siblings, _ := filepath.Glob(filepath.Join(filepath.Dir(root), "*", TrackersFile))
	return siblings
}

// oneEachRepository keeps one trackers document per git repository among
// files: worktrees share their repository's common git directory, and the
// main checkout's file is the one kept, since a worktree's branch may carry
// an unmerged change to it.
func oneEachRepository(files []string, checkoutOf func(string) string) []string {
	var out []string
	kept := map[string]int{}
	for _, f := range files {
		checkout := checkoutOf(f)
		common := commonDir(checkout)
		main := filepath.Clean(filepath.Dir(common)) == filepath.Clean(checkout)
		if at, seen := kept[common]; seen {
			if main {
				out[at] = f
			}
			continue
		}
		kept[common] = len(out)
		out = append(out, f)
	}
	return out
}

// commonDir is the git directory a checkout's repository shares among its
// worktrees, absolute; the checkout itself when git cannot say.
func commonDir(checkout string) string {
	if b, err := exec.Command("git", "-C", checkout, "rev-parse", "--path-format=absolute", "--git-common-dir").Output(); err == nil {
		return strings.TrimSpace(string(b))
	}
	return checkout
}

// Siblings is the checkouts beside dir's repository, as repositories sit
// side by side under their owner: one per repository, a repository's
// worktrees read from its main checkout, and dir's own repository left out.
func Siblings(dir string) []string {
	root := dir
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
	}
	own := commonDir(root)
	gits, _ := filepath.Glob(filepath.Join(filepath.Dir(root), "*", ".git"))
	var out []string
	for _, g := range oneEachRepository(gits, filepath.Dir) {
		if c := filepath.Dir(g); commonDir(c) != own {
			out = append(out, c)
		}
	}
	return out
}

// RecordedGroup is the program group the trackers document for the checkout
// at dir records, the document FindTrackers finds; empty when there is
// none, it does not parse, or it records no group (config format trackers,
// GH-103). It reads the one field itself, so the words that need the group
// do not reach for the tracking systems.
func RecordedGroup(dir string) string {
	path, err := FindTrackers(dir)
	if err != nil || path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var f struct {
		Group string `yaml:"group"`
	}
	if corpus.DecodeConfig(b, &f) != nil {
		return ""
	}
	return strings.TrimSpace(f.Group)
}
