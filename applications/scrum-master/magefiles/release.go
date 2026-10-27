// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
)

// repoRuntime is the runtime every gate and rig target runs against, as
// runtime.Resolve finds it for this application: the sibling agent-core
// inside the repository that ships it (GH-190), else the parent checkout
// or pinned release the application names (GH-2754).
func repoRuntime() (runtime.Release, error) {
	root, err := repositoryRoot()
	if err != nil {
		return runtime.Release{}, err
	}
	return runtime.Resolve(root, runStreaming)
}

// withCheckouts writes docs/external-checkouts.yaml in root pointing each
// parent repository at src while the audit runs, and returns what removes it.
// A checkouts file the person wrote is left as it is.
func withCheckouts(root, src string, repos ...string) (func(), error) {
	path := filepath.Join(root, checkoutsFile)
	if _, err := os.Stat(path); err == nil {
		return func() {}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("# Written by mage audit for this run; it points at the downloaded release.\ncheckouts:\n")
	for _, repo := range repos {
		fmt.Fprintf(&b, "  %s: %s\n", repo, src)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	return func() { os.Remove(path) }, nil
}
