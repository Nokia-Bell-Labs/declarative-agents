// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package runtime is what scrum-master knows of the declarative-agents
// runtime: the release a new corpus pins, the runtime this repository holds
// beside the application, and how the agent is built with the words
// (GH-27). A program installs the agent and the profiles with install.sh;
// nothing here fetches them (GH-34).
package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/spectool"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
)

// fallbackPin is the newest release of this repository with published agent
// binaries, written into a new corpus when the words are not themselves a
// release build; a release writes its own version, since the words and the
// runtime ship from one repository (GH-190, srd043 R3.3).
const fallbackPin = "v0.20261018.1"

// DefaultPin is the release a new corpus pins, the declarative-agents
// release whose format and corpus audit it follows: the words' own release,
// else the newest release with published binaries (GH-190).
func DefaultPin() string {
	if v := version.Current(); version.IsRelease(v) {
		return v
	}
	return fallbackPin
}

// Sibling is the runtime this repository holds beside the application, when
// dir is inside the repository that ships agent-core: the repository root
// is the source and the agent builds from agent-core into agent-core/bin —
// no pin and no download, the sibling is the release (GH-190) — composed
// with this application's words, which it serves in process (GH-27). With a nil
// builder it only locates; ok reports whether dir is in that repository.
func Sibling(dir string, build func(dir, name string, args ...string) error) (Release, bool, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return Release{}, false, nil
	}
	root := strings.TrimSpace(string(out))
	core := filepath.Join(root, "agent-core")
	if _, err := os.Stat(filepath.Join(core, "cmd", "agent")); err != nil {
		return Release{}, false, nil
	}
	// Build products live in the release cache, not in the tree: a scratch
	// module inside the repository would read as a maintained module.
	dir, derr := spectool.ReleaseDir("in-repo")
	if derr != nil {
		return Release{}, true, derr
	}
	r := Release{Tag: "in-repo", Dir: dir, Src: root, Agent: filepath.Join(dir, "agent")}
	if build == nil {
		return r, true, nil
	}
	comp, err := Compose(core, filepath.Join(root, "applications", "scrum-master"))
	if err != nil {
		return r, true, err
	}
	defer comp.Cleanup()
	// build takes no environment, so env(1) carries the workspace.
	args := append(append(append(comp.Env, "go", "build"), comp.Flags...), "-o", r.Agent, "./cmd/agent")
	if err := build(comp.Dir, "env", args...); err != nil {
		return r, true, err
	}
	return r, true, nil
}

// Release is a release's runtime on this host: its directory under the
// user cache directory, its source tree (a git repository holding the tree
// at the tag), and its agent.
type Release struct {
	Tag, Dir, Src, Agent string
}

// CoreRoot is the agent-core tree a profile's paths resolve against: the
// agent's --core-root.
func (r Release) CoreRoot() string { return filepath.Join(r.Src, "agent-core") }

// Built reports whether the spec tool is already built in tag's directory.
func Built(tag string) bool {
	dir, err := spectool.ReleaseDir(tag)
	if err != nil {
		return false
	}
	_, err = os.Stat(spectool.Binary(dir))
	return err == nil
}
