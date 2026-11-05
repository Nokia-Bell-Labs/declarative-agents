// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
)

// VendorDir is where a program repository keeps the profiles its skills
// run and the agent-core declarations they read (GH-29). It is committed,
// so a repository runs the profiles it has checked in.
const VendorDir = ".scrum-master"

// AgentsVendored and CoreVendored are the vendored trees under VendorDir: a
// skill runs agent --profile <AgentsVendored>/<name>/profile.yaml
// --core-root <CoreVendored>.
const (
	AgentsVendored = VendorDir + "/agents"
	CoreVendored   = VendorDir + "/agent-core"
)

// ManifestFile records what was vendored: the release, and each file's
// digest as written, so a rerun tells an edited file from an outdated one.
const ManifestFile = VendorDir + "/manifest.json"

// Manifest is ManifestFile's content.
type Manifest struct {
	Release string `json:"release"`
	// Runtime is the runtime release the vendoring agent was built
	// against, the release a corpus pins (GH-2771); empty in a manifest
	// written before it was recorded.
	Runtime string            `json:"runtime,omitempty"`
	Files   map[string]string `json:"files"`
}

// VendorSource is the release a vendoring copies from: its name, its
// profiles directory, and its agent-core tools directory.
type VendorSource struct {
	Release, Agents, CoreTools string
}

// Vendor copies the release's profiles and agent-core tools into dir's
// VendorDir and writes the manifest. A file it wrote and nobody edited is
// rewritten when the release differs; an edited file, or one it did not
// write, is kept unless force; a vendored file the release no longer has is
// removed when unedited. A dry run writes nothing. A rerun on an up-to-date
// repository reports every file unchanged. A release the machine should not
// vendor is refused, writing nothing (CheckRelease).
func Vendor(dir string, src VendorSource, force, dryRun bool) ([]Result, error) {
	if err := CheckRelease(dir, src); err != nil {
		return nil, fmt.Errorf("%w; nothing was written", err)
	}
	want, err := vendorTree(src)
	if err != nil {
		return nil, err
	}
	old, err := readManifest(dir)
	if err != nil {
		return nil, err
	}
	next := Manifest{Release: src.Release, Runtime: runtime.DefaultPin(), Files: map[string]string{}}
	var results []Result
	for _, rel := range sortedKeys(want) {
		r, sum, err := vendorFile(dir, rel, want[rel], old.Files[rel], force, dryRun)
		if err != nil {
			return nil, err
		}
		next.Files[rel] = sum
		results = append(results, r)
	}
	for _, rel := range sortedKeys(old.Files) {
		if _, ok := want[rel]; ok {
			continue
		}
		path := filepath.Join(dir, filepath.FromSlash(rel))
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		if digest(string(b)) != old.Files[rel] && !force {
			results = append(results, Result{Path: path, Action: "kept", Reason: "edited, and the release no longer has it"})
			next.Files[rel] = old.Files[rel]
			continue
		}
		if !dryRun {
			if err := os.Remove(path); err != nil {
				return nil, err
			}
		}
		results = append(results, Result{Path: path, Action: "removed"})
	}
	if err := writeManifest(dir, next, dryRun); err != nil {
		return nil, err
	}
	return results, nil
}

// vendorFile writes one vendored file and returns its result and the digest
// the manifest records: what is on disk afterwards, or, for a kept edited
// file, what was vendored, so it reads as edited again.
func vendorFile(dir, rel string, content []byte, vendored string, force, dryRun bool) (Result, string, error) {
	path := filepath.Join(dir, filepath.FromSlash(rel))
	sum := digest(string(content))
	have, err := os.ReadFile(path)
	action := "created"
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Result{}, "", err
	case bytes.Equal(have, content):
		return Result{Path: path, Action: "unchanged"}, sum, nil
	case digest(string(have)) != vendored && !force:
		reason := "edited since it was vendored; kept"
		if vendored == "" {
			reason = "not vendored by scrum-master; kept"
		}
		return Result{Path: path, Action: "kept", Reason: reason}, vendored, nil
	default:
		action = "updated"
	}
	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, "", err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return Result{}, "", err
		}
	}
	return Result{Path: path, Action: action}, sum, nil
}

// vendorTree reads the release's two trees as the files VendorDir holds,
// keyed by their slash path under dir.
func vendorTree(src VendorSource) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, t := range []struct{ from, to string }{{src.Agents, AgentsVendored}, {src.CoreTools, CoreVendored + "/tools"}} {
		err := filepath.WalkDir(t.from, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(t.from, path)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[t.to+"/"+filepath.ToSlash(rel)] = b
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readManifest(dir string) (Manifest, error) {
	m := Manifest{Files: map[string]string{}}
	b, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	} else if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	return m, nil
}

func writeManifest(dir string, m Manifest, dryRun bool) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil || dryRun {
		return err
	}
	path := filepath.Join(dir, ManifestFile)
	if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, append(b, '\n')) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// VendoredRelease is the release the repository at dir vendors, from its
// manifest; empty when it vendors none (GH-2813).
func VendoredRelease(dir string) (string, error) {
	m, err := readManifest(dir)
	return m.Release, err
}

// CheckRelease refuses a release this machine should not vendor into the
// repository at dir (GH-2899): one older than the repository already vendors,
// which a second machine that pulled before it installed would otherwise take
// the repository back to, or one newer than the running agent, as when PATH
// finds an older agent than install.sh installed. Going back on purpose is a
// revert of the upgrade's merge. A development build or a source tree is no
// release and is never refused.
func CheckRelease(dir string, src VendorSource) error {
	m, err := readManifest(dir)
	if err != nil {
		return err
	}
	if version.Older(src.Release, m.Release) {
		return fmt.Errorf("%s vendors %s, newer than the release %s installed on this machine; run install.sh %s here, or revert the upgrade's merge to go back on purpose", VendorDir, m.Release, src.Release, m.Release)
	}
	if cur := version.Current(); version.Older(cur, src.Release) {
		return fmt.Errorf("this agent is %s, older than the release %s it would vendor; run install.sh %s, and check which agent PATH finds", cur, src.Release, src.Release)
	}
	return nil
}

// StaleAgent says why the running agent is too old for the repository at dir,
// empty when it is not: older than the release the repository vendors, as on
// a machine that pulled the upgrade without running install.sh (GH-2899).
func StaleAgent(dir string) string {
	m, err := readManifest(dir)
	if err != nil {
		return ""
	}
	if cur := version.Current(); version.Older(cur, m.Release) {
		return fmt.Sprintf("this agent is %s, but %s vendors %s; run install.sh %s, and check which agent PATH finds", cur, VendorDir, m.Release, m.Release)
	}
	return ""
}

// StaleAgentAt is StaleAgent for the repository holding dir, found up its
// folders with no process started, so every word can afford to ask.
func StaleAgentAt(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
			return StaleAgent(dir)
		}
		parent := filepath.Dir(dir)
		_, git := os.Stat(filepath.Join(dir, ".git"))
		if git == nil || parent == dir {
			return ""
		}
		dir = parent
	}
}

// CheckVendored reports each vendored file that differs from the manifest:
// edited, since its digest is not the one vendored, or missing. It reads no
// release, so a repository checks the same wherever it runs (GH-35). A
// repository that vendored nothing is an error.
func CheckVendored(dir string) ([]Finding, error) {
	b, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.New("nothing is vendored in " + VendorDir + "; run the upgrade profile")
	} else if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	var findings []Finding
	for _, rel := range sortedKeys(m.Files) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		have, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			findings = append(findings, Finding{Path: path, Problem: "missing"})
		case err != nil:
			return nil, err
		case digest(string(have)) != m.Files[rel]:
			findings = append(findings, Finding{Path: path, Problem: "edited"})
		}
	}
	return findings, nil
}
