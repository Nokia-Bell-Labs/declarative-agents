// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
)

// UIKit builds, tests, packs, and prepares releases of the shared UI kit
// package, @declarative-agents/ui-kit (srd004 R8).
type UIKit mg.Namespace

const (
	uiKitDir         = "applications/ui-kit"
	uiKitPackageName = "@declarative-agents/ui-kit"
	uiKitOutDir      = "out"
	uiKitTagPrefix   = "ui-kit/v"
	uiKitReleaseRepo = "Nokia-Bell-Labs/declarative-agents"
)

// Build installs the kit's locked dependencies and builds its library dist.
func (UIKit) Build() error {
	return uiKitBuild(uiKitDir, runIn)
}

// Test installs the kit's locked dependencies, type-checks, and runs its tests.
func (UIKit) Test() error {
	return uiKitTest(uiKitDir, runIn)
}

// Pack builds the kit and writes its npm tarball to applications/ui-kit/out.
func (UIKit) Pack() error {
	_, err := uiKitPack(uiKitDir, runIn)
	return err
}

// Release packs the kit from a clean tree and prints the commands that publish
// it as the GitHub release ui-kit/vX.Y.Z. It creates no tag and no release.
func (UIKit) Release() error {
	if err := requireCleanTree(); err != nil {
		return err
	}
	tarball, err := uiKitPack(uiKitDir, runIn)
	if err != nil {
		return err
	}
	version, err := uiKitVersion(uiKitDir)
	if err != nil {
		return err
	}
	head, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	tag := uiKitTagPrefix + version
	tagged, _ := gitOutput("rev-parse", "--verify", "--quiet", tag+"^{commit}")
	if err := checkUIKitTag(tag, tagged, head); err != nil {
		return err
	}
	fmt.Print(uiKitReleaseCommands(tag, tagged == "", tarball))
	return nil
}

func uiKitInstall(dir string, run uiRunner) error {
	if err := run(dir, "npm", "ci", "--no-audit", "--prefer-offline"); err != nil {
		return fmt.Errorf("%s: npm ci failed: %w", dir, err)
	}
	return nil
}

func uiKitBuild(dir string, run uiRunner) error {
	if err := uiKitInstall(dir, run); err != nil {
		return err
	}
	if err := run(dir, "npm", "run", "build"); err != nil {
		return fmt.Errorf("%s: npm run build failed: %w", dir, err)
	}
	return nil
}

func uiKitTest(dir string, run uiRunner) error {
	if err := uiKitInstall(dir, run); err != nil {
		return err
	}
	if err := run(dir, "npm", "test"); err != nil {
		return fmt.Errorf("%s: npm test failed: %w", dir, err)
	}
	return nil
}

// uiKitPack builds the kit and returns the path of the packed tarball.
func uiKitPack(dir string, run uiRunner) (string, error) {
	if err := uiKitBuild(dir, run); err != nil {
		return "", err
	}
	out := filepath.Join(dir, uiKitOutDir)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	if err := run(dir, "npm", "pack", "--pack-destination", uiKitOutDir); err != nil {
		return "", fmt.Errorf("%s: npm pack failed: %w", dir, err)
	}
	version, err := uiKitVersion(dir)
	if err != nil {
		return "", err
	}
	tarball := filepath.Join(out, uiKitTarballName(version))
	if _, err := os.Stat(tarball); err != nil {
		return "", fmt.Errorf("npm pack did not produce %s: %w", tarball, err)
	}
	return tarball, nil
}

// uiKitTarballName is the file name npm pack gives a scoped package.
func uiKitTarballName(version string) string {
	name := strings.ReplaceAll(strings.TrimPrefix(uiKitPackageName, "@"), "/", "-")
	return name + "-" + version + ".tgz"
}

func uiKitVersion(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", err
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", fmt.Errorf("%s: parse package.json: %w", dir, err)
	}
	if pkg.Name != uiKitPackageName {
		return "", fmt.Errorf("%s: package name %q, want %q", dir, pkg.Name, uiKitPackageName)
	}
	if strings.TrimSpace(pkg.Version) == "" {
		return "", fmt.Errorf("%s: package.json has no version", dir)
	}
	return pkg.Version, nil
}

// checkUIKitTag accepts an absent tag or one that already names HEAD; a tag on
// another commit means package.json was not bumped for this release.
func checkUIKitTag(tag, taggedCommit, head string) error {
	if taggedCommit == "" || taggedCommit == head {
		return nil
	}
	return fmt.Errorf("%s already tags %s, not HEAD %s; bump the version in %s/package.json",
		tag, taggedCommit, head, uiKitDir)
}

func uiKitReleaseCommands(tag string, needsTag bool, tarball string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ui-kit release %s is ready. Publish it with:\n", tag)
	if needsTag {
		fmt.Fprintf(&b, "  git tag %s\n  git push origin %s\n", tag, tag)
	}
	fmt.Fprintf(&b, "  gh release create %s %s --repo %s --title %q --notes %q\n",
		tag, tarball, uiKitReleaseRepo, tag, "UI kit "+strings.TrimPrefix(tag, uiKitTagPrefix))
	fmt.Fprintf(&b, "Consumers depend on https://github.com/%s/releases/download/%s/%s\n",
		uiKitReleaseRepo, strings.ReplaceAll(tag, "/", "%2F"), filepath.Base(tarball))
	return b.String()
}

func requireCleanTree() error {
	status, err := gitOutput("status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("ui-kit release requires a clean working tree:\n%s", status)
	}
	return nil
}

// uiKitDependent reports whether the UI package at dir depends on the kit.
func uiKitDependent(dir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false, fmt.Errorf("%s: read package.json: %w", dir, err)
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, fmt.Errorf("%s: parse package.json: %w", dir, err)
	}
	_, dep := pkg.Dependencies[uiKitPackageName]
	_, dev := pkg.DevDependencies[uiKitPackageName]
	return dep || dev, nil
}

// testUIKit runs the kit's tests under the same npm policy as UIDist: a clean
// skip without npm, a failure without npm during a release gate.
func testUIKit() error {
	proceed, err := uiDistPrerequisite(exec.LookPath, releaseModeEnabled())
	if err != nil || !proceed {
		return err
	}
	fmt.Printf("=== %s tests ===\n", uiKitDir)
	return uiKitTest(uiKitDir, runIn)
}

// embeddedUIBundle pairs a kit-built SPA with the go:embed directory agent-core
// compiles it from (srd029 R5.9, applications srd004 R9). The built dist is
// committed there because agent-core is its own Go module.
type embeddedUIBundle struct {
	source   string
	embedded string
}

var embeddedUIBundles = []embeddedUIBundle{
	{source: "applications/ui-kit/observer", embedded: "agent-core/internal/tools/rest/bundles/observer"},
}

// Observer builds the kit and the observer SPA and replaces the embedded
// observer bundle in agent-core with the fresh dist.
func (UIKit) Observer() error {
	return buildEmbeddedBundle(embeddedUIBundles[0], runIn)
}

func buildEmbeddedBundle(bundle embeddedUIBundle, run uiRunner) error {
	if err := uiKitBuild(uiKitDir, run); err != nil {
		return err
	}
	if err := run(bundle.source, "npm", "ci", "--no-audit", "--prefer-offline"); err != nil {
		return fmt.Errorf("%s: npm ci failed: %w", bundle.source, err)
	}
	if err := run(bundle.source, "npm", "run", "build"); err != nil {
		return fmt.Errorf("%s: npm run build failed: %w", bundle.source, err)
	}
	if err := os.RemoveAll(bundle.embedded); err != nil {
		return err
	}
	return copyDirExcluding(filepath.Join(bundle.source, "dist"), bundle.embedded, nil)
}
