// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package gcprig

import (
	"fmt"
	"os/exec"
	"strings"
)

// CommandRunner executes one command and returns its combined output; the
// tests substitute a recorder, mirroring kindrig.
type CommandRunner func(name string, args ...string) ([]byte, error)

// DefaultRun executes commands in the current environment.
func DefaultRun(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// LookPath is swapped in tests so preflight is provable on a machine that
// has gcloud as well as one that does not.
var LookPath = exec.LookPath

// Preflight verifies the rig can act at all: gcloud present, an account
// active, the configured project set and reachable. Provisioning is always
// operator-requested, so every failure is an error with its one-line
// remedy, never a skip (eng08).
func Preflight(run CommandRunner, config Config) error {
	if _, err := LookPath("gcloud"); err != nil {
		return fmt.Errorf("gcloud is not installed; install the Google Cloud CLI: " +
			"https://cloud.google.com/sdk/docs/install")
	}
	if strings.TrimSpace(config.Project) == "" {
		return fmt.Errorf("no project configured; set project in %s beside the repository root", ConfigFile)
	}
	out, err := run("gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
	if err != nil {
		return fmt.Errorf("gcloud auth list: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("no active gcloud account; run: gcloud auth login")
	}
	if out, err := run("gcloud", "projects", "describe", config.Project,
		"--format=value(projectId)"); err != nil {
		return fmt.Errorf("project %q is not accessible to the active account: %s",
			config.Project, strings.TrimSpace(string(out)))
	}
	return nil
}
