// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// uiSearchRoots are the trees holding shipped profile UIs whose built dist is
// checked in and served directly by a profile. agent-core rebuilds its own
// embedded UIs before compilation, so it is not scanned here (GH-518).
var uiSearchRoots = []string{"agent-profiles/agents", "examples"}

// UIDist rebuilds every shipped profile UI from source with a clean,
// lockfile-pinned install (npm ci) and fails when the tracked dist differs from
// the build output, so a served bundle cannot silently diverge from its source
// (GH-518). It skips cleanly when npm is unavailable.
func UIDist() error {
	if _, err := exec.LookPath("npm"); err != nil {
		fmt.Println("SKIP uiDist: npm not found; the UI reproducibility gate needs node/npm")
		return nil
	}
	uis, err := discoverShippedUIs(uiSearchRoots)
	if err != nil {
		return err
	}
	if len(uis) == 0 {
		return fmt.Errorf("uiDist found no shipped UIs to check under %v", uiSearchRoots)
	}
	for _, dir := range uis {
		fmt.Printf("=== ui reproducibility: %s ===\n", dir)
		if err := rebuildAndDiffUI(dir); err != nil {
			return err
		}
	}
	fmt.Printf("uiDist PASS: %d shipped UI dist tree(s) reproduce from a clean source build\n", len(uis))
	return nil
}

// discoverShippedUIs returns each app directory under the search roots that has a
// package-lock.json (pinned tooling), a build script, and a tracked dist tree.
func discoverShippedUIs(roots []string) ([]string, error) {
	var uis []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() != "package-lock.json" {
				return nil
			}
			appDir := filepath.Dir(path)
			if hasBuildScript(appDir) && isDir(filepath.Join(appDir, "dist")) {
				uis = append(uis, appDir)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Strings(uis)
	return uis, nil
}

func hasBuildScript(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	return strings.TrimSpace(pkg.Scripts["build"]) != ""
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// rebuildAndDiffUI copies the app's source into a temp dir, runs a clean
// install and build, and byte-compares the produced dist with the tracked one.
func rebuildAndDiffUI(appDir string) error {
	tmp, err := os.MkdirTemp("", "uidist-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	build := filepath.Join(tmp, "app")
	if err := copyDirExcluding(appDir, build, map[string]bool{"node_modules": true, "dist": true}); err != nil {
		return err
	}
	if err := runIn(build, "npm", "ci"); err != nil {
		return fmt.Errorf("%s: npm ci failed: %w", appDir, err)
	}
	if err := runIn(build, "npm", "run", "build"); err != nil {
		return fmt.Errorf("%s: npm run build failed: %w", appDir, err)
	}
	if diff := diffTrees(filepath.Join(appDir, "dist"), filepath.Join(build, "dist")); diff != "" {
		return fmt.Errorf("%s: tracked dist differs from a clean source build; rebuild and commit dist:\n%s", appDir, diff)
	}
	return nil
}

func runIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyDirExcluding(src, dst string, skip map[string]bool) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(path, filepath.Join(dst, rel))
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}

// diffTrees returns a human-readable description of the first differences
// between two directory trees (missing/extra files, or differing bytes), or the
// empty string when they are identical.
func diffTrees(a, b string) string {
	fa, err := treeFiles(a)
	if err != nil {
		return fmt.Sprintf("read %s: %v", a, err)
	}
	fb, err := treeFiles(b)
	if err != nil {
		return fmt.Sprintf("read %s: %v", b, err)
	}
	var diffs []string
	for rel := range fa {
		if _, ok := fb[rel]; !ok {
			diffs = append(diffs, "  only in tracked dist: "+rel)
			continue
		}
		da, _ := os.ReadFile(filepath.Join(a, rel))
		db, _ := os.ReadFile(filepath.Join(b, rel))
		if !bytes.Equal(da, db) {
			diffs = append(diffs, fmt.Sprintf("  content differs: %s (%d vs %d bytes)", rel, len(da), len(db)))
		}
	}
	for rel := range fb {
		if _, ok := fa[rel]; !ok {
			diffs = append(diffs, "  only in rebuilt dist: "+rel)
		}
	}
	sort.Strings(diffs)
	return strings.Join(diffs, "\n")
}

func treeFiles(root string) (map[string]bool, error) {
	out := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[rel] = true
		return nil
	})
	return out, err
}
