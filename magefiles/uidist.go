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

// uiSearchRoots are the catalog and runnable application trees holding shipped
// UIs whose built dist is checked in. agent-core rebuilds its own embedded UIs
// before compilation, so it is not scanned here (GH-518, GH-1046).
var uiSearchRoots = []string{
	"applications/catalog",
	"applications/coding-agent",
	"applications/chatbot-mesh",
}

const (
	uiAuditLevel          = "high"
	canonicalUITokensPath = "applications/catalog/ui/design-tokens.css"
)

// uiDistReleaseEnv is set by the release gate so UIDist treats a missing npm as
// a fatal gate failure instead of a developer-convenience skip. Outside a
// release (a plain `mage test` on a machine without node/npm), the gate skips
// cleanly so day-to-day Go work is not blocked (GH-1349).
const uiDistReleaseEnv = "DA_RELEASE_GATE"

type uiRunner func(string, string, ...string) error

// releaseModeEnabled reports whether UIDist is running as part of a release
// gate, in which case skipping a required prerequisite is not permitted.
func releaseModeEnabled() bool {
	return strings.TrimSpace(os.Getenv(uiDistReleaseEnv)) != ""
}

// uiDistPrerequisite decides whether the UI reproducibility gate can run given
// npm availability and whether this is a release gate. It returns proceed=true
// when npm is present; a fatal error when npm is absent during a release
// (the gate must not silently pass without rebuilding shipped UIs and auditing
// their dependencies); and proceed=false with no error when npm is absent
// outside a release (a clean developer skip). The prerequisite lookup is
// injected so both policies are testable at the orchestration level.
func uiDistPrerequisite(lookPath func(string) (string, error), releaseMode bool) (bool, error) {
	if _, err := lookPath("npm"); err == nil {
		return true, nil
	}
	if releaseMode {
		return false, fmt.Errorf(
			"uiDist: npm not found but %s is set; the release requires rebuilding shipped "+
				"UIs and auditing their dependencies — install node/npm on the release runner",
			uiDistReleaseEnv)
	}
	fmt.Println("SKIP uiDist: npm not found; the UI reproducibility gate needs node/npm")
	return false, nil
}

// UIDist rebuilds every shipped profile UI from source with a clean,
// lockfile-pinned install (npm ci), gates both full build-chain and
// production-only dependencies at high severity, and fails when the tracked
// dist differs from the build output (GH-518, GH-1003). Missing npm is fatal
// during a release gate and a clean skip otherwise (GH-1349).
func UIDist() error {
	proceed, err := uiDistPrerequisite(exec.LookPath, releaseModeEnabled())
	if err != nil {
		return err
	}
	if !proceed {
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

// discoverShippedUIs returns each locked UI package under the search roots.
// Every such package is shipped and therefore must expose the common script
// vocabulary and a tracked dist tree; silently omitting a malformed package
// would make the reproducibility gate pass without checking all shipped UIs.
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
			pkg, err := readUIPackage(appDir)
			if err != nil {
				return err
			}
			for _, script := range []string{"dev", "build", "preview"} {
				if strings.TrimSpace(pkg.Scripts[script]) == "" {
					return fmt.Errorf("%s: shipped UI package is missing required %q script", appDir, script)
				}
			}
			if !isDir(filepath.Join(appDir, "dist")) {
				return fmt.Errorf("%s: shipped UI package has no tracked dist tree", appDir)
			}
			uis = append(uis, appDir)
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Strings(uis)
	return uis, nil
}

type uiPackage struct {
	Scripts map[string]string `json:"scripts"`
}

func readUIPackage(dir string) (uiPackage, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return uiPackage{}, fmt.Errorf("%s: read package.json: %w", dir, err)
	}
	var pkg uiPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return uiPackage{}, fmt.Errorf("%s: parse package.json: %w", dir, err)
	}
	return pkg, nil
}

func hasTestScript(dir string) (bool, error) {
	pkg, err := readUIPackage(dir)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(pkg.Scripts["test"]) != "", nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// rebuildAndDiffUI copies the app's source into a temp dir, runs a clean
// install and build, and byte-compares the produced dist with the tracked one.
func rebuildAndDiffUI(appDir string) error {
	return rebuildAndDiffUIWithRunner(appDir, runIn)
}

func rebuildAndDiffUIWithRunner(appDir string, run uiRunner) error {
	tmp, err := os.MkdirTemp("", "uidist-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	build, err := stageUIBuild(appDir, tmp)
	if err != nil {
		return err
	}
	if err := run(build, "npm", "ci"); err != nil {
		return fmt.Errorf("%s: npm ci failed: %w", appDir, err)
	}
	if err := auditUIDependencies(build, run); err != nil {
		return fmt.Errorf("%s: %w", appDir, err)
	}
	hasTest, err := hasTestScript(build)
	if err != nil {
		return fmt.Errorf("%s: %w", appDir, err)
	}
	if hasTest {
		if err := run(build, "npm", "test"); err != nil {
			return fmt.Errorf("%s: npm test failed: %w", appDir, err)
		}
	}
	if err := run(build, "npm", "run", "build"); err != nil {
		return fmt.Errorf("%s: npm run build failed: %w", appDir, err)
	}
	if diff := diffTrees(filepath.Join(appDir, "dist"), filepath.Join(build, "dist")); diff != "" {
		return fmt.Errorf("%s: tracked dist differs from a clean source build; rebuild and commit dist:\n%s", appDir, diff)
	}
	return nil
}

// stageUIBuild preserves a package's repository-relative location and stages
// the canonical token source beside it. Relative CSS imports therefore resolve
// identically in a clean gate build and in the source checkout, while packaged
// closures continue to consume the compiled token CSS from their tracked dist.
func stageUIBuild(appDir, tmp string) (string, error) {
	absApp, err := filepath.Abs(appDir)
	if err != nil {
		return "", err
	}
	repoRoot, rel, ok := uiRepositoryLayout(absApp)
	if !ok {
		build := filepath.Join(tmp, "app")
		return build, copyDirExcluding(appDir, build, map[string]bool{"node_modules": true, "dist": true})
	}

	buildRepo := filepath.Join(tmp, "repo")
	build := filepath.Join(buildRepo, rel)
	if err := copyDirExcluding(appDir, build, map[string]bool{"node_modules": true, "dist": true}); err != nil {
		return "", err
	}
	if err := copyFile(
		filepath.Join(repoRoot, filepath.FromSlash(canonicalUITokensPath)),
		filepath.Join(buildRepo, filepath.FromSlash(canonicalUITokensPath)),
	); err != nil {
		return "", fmt.Errorf("stage canonical UI tokens: %w", err)
	}
	return build, nil
}

func uiRepositoryLayout(absApp string) (root, rel string, ok bool) {
	for candidate := absApp; ; candidate = filepath.Dir(candidate) {
		if info, err := os.Stat(filepath.Join(candidate, filepath.FromSlash(canonicalUITokensPath))); err == nil && !info.IsDir() {
			rel, err := filepath.Rel(candidate, absApp)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return candidate, rel, true
			}
			return "", "", false
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", "", false
		}
	}
}

func auditUIDependencies(dir string, run uiRunner) error {
	scopes := []struct {
		name string
		args []string
	}{
		{name: "full build dependency", args: []string{
			"audit", "--audit-level=" + uiAuditLevel}},
		{name: "production-only dependency", args: []string{
			"audit", "--omit=dev", "--audit-level=" + uiAuditLevel}},
	}
	for _, scope := range scopes {
		if err := run(dir, "npm", scope.args...); err != nil {
			return fmt.Errorf(
				"%s audit failed (policy: zero high/critical vulnerabilities): %w",
				scope.name, err)
		}
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
