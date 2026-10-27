// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package runtime is what scrum-master knows of the declarative-agents
// runtime: the release a new corpus pins, the runtime this repository holds
// beside the application, and how the agent is built with the words
// (GH-27). A program installs the agent and the profiles with install.sh;
// nothing here fetches them (GH-34).
package runtime

import (
	"fmt"
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
	cache, derr := spectool.ReleaseDir("in-repo")
	if derr != nil {
		return Release{}, true, derr
	}
	r := Release{Tag: "in-repo", Dir: cache, Src: root, Agent: filepath.Join(cache, "agent")}
	if build == nil {
		return r, true, nil
	}
	return r, true, r.build(appRoot(dir, filepath.Join(root, "applications", "scrum-master")), build)
}

// build composes the agent from the release's agent-core with the words of
// the application at app, into r.Agent.
func (r Release) build(app string, build func(dir, name string, args ...string) error) error {
	comp, err := Compose(filepath.Join(r.Src, "agent-core"), app)
	if err != nil {
		return err
	}
	defer comp.Cleanup()
	// build takes no environment, so env(1) carries the workspace.
	args := append(append(append(comp.Env, "go", "build"), comp.Flags...), "-o", r.Agent, "./cmd/agent")
	return build(comp.Dir, "env", args...)
}

// appRoot is the application module holding dir: the nearest folder above
// it with a go.mod naming this application's words, else fallback.
func appRoot(dir, fallback string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fallback
	}
	for cur := abs; ; cur = filepath.Dir(cur) {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(cur, "words")); err == nil {
				return cur
			}
		}
		if filepath.Dir(cur) == cur {
			return fallback
		}
	}
}

// Resolve is the runtime the application at app builds and gates
// against, resolved in one place (GH-2754): the agent-core beside it in
// the repository that ships agent-core, as before (Sibling); else the
// parent checkout docs/external-checkouts.yaml names; else the
// declarative-agents release docs/external-pins.yaml pins, its source laid
// out once per tag as a pinned corpus's is. With a nil builder it only
// locates; otherwise it composes the agent with the application's words.
func Resolve(app string, build func(dir, name string, args ...string) error) (Release, error) {
	if r, ok, err := Sibling(app, build); ok {
		return r, err
	}
	var r Release
	if src, ok, err := spectool.Checkout(app); err != nil {
		return r, err
	} else if ok {
		cache, err := spectool.ReleaseDir("checkout")
		if err != nil {
			return r, err
		}
		r = Release{Tag: "checkout", Dir: cache, Src: src, Agent: filepath.Join(cache, "agent")}
	} else {
		tag, err := spectool.Pinned(app)
		if err != nil {
			return r, fmt.Errorf("no runtime for %s: no agent-core beside it, no checkout of %s in %s, and no pin in %s: %w", app, spectool.ParentRepository, spectool.CheckoutsFile, spectool.PinsFile, err)
		}
		cache, err := spectool.ReleaseDir(tag)
		if err != nil {
			return r, err
		}
		src := filepath.Join(cache, "src")
		if err := spectool.EnsureSourceFrom(spectool.SourceURL(app), src, tag); err != nil {
			return r, err
		}
		r = Release{Tag: tag, Dir: cache, Src: src, Agent: filepath.Join(cache, "agent-composed")}
	}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return r, err
	}
	if build == nil {
		return r, nil
	}
	return r, r.build(app, build)
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
