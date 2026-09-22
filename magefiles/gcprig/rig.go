// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package gcprig

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// Up provisions or reuses everything Table 1 of eng08 names, in dependency
// order, and leaves it running.
func Up(run CommandRunner, config Config) error {
	if err := Preflight(run, config); err != nil {
		return err
	}
	if err := EnsureCluster(run, config); err != nil {
		return err
	}
	for _, application := range config.FirstPartyApplications() {
		if err := EnsureBucket(run, application); err != nil {
			return err
		}
		if err := EnsureIdentity(run, application); err != nil {
			return err
		}
	}
	if err := EnsureRegistry(run, config); err != nil {
		return err
	}
	if err := EnsureRegistryAccess(run, config); err != nil {
		return err
	}
	kindrig.LogPhase(config.Cluster, "gcp-up", "complete", time.Now(),
		"application-buckets=3 registry="+config.RegistryPath())
	return nil
}

// DownConfirmation is the exact argument gcp:down requires. A teardown in a
// shared project deletes real resources; the confirmation is typed, not
// defaulted.
const DownConfirmation = "delete-gcp-rig"

// Down deletes platform compute and the image registry, cluster last, behind
// the typed confirmation. Application buckets and workload identities are
// retained so cluster recreation reattaches without turning platform teardown
// into application data deletion.
func Down(run CommandRunner, config Config, confirmation string) error {
	if confirmation != DownConfirmation {
		return fmt.Errorf("gcp:down deletes cluster %q and registry %s in project %s; "+
			"application buckets and workload identities are retained; run again with the confirmation argument %q",
			config.Cluster, config.Registry,
			config.Project, DownConfirmation)
	}
	if err := Preflight(run, config); err != nil {
		return err
	}
	deletions := []struct {
		name string
		args []string
	}{
		{"registry", []string{"gcloud", "artifacts", "repositories", "delete", config.Registry,
			"--project", config.Project, "--location", config.Region, "--quiet"}},
		{"cluster", []string{"gcloud", "container", "clusters", "delete", config.Cluster,
			"--project", config.Project, "--region", config.Region, "--quiet"}},
	}
	for _, deletion := range deletions {
		started := time.Now()
		if out, err := run(deletion.args[0], deletion.args[1:]...); err != nil {
			if notFound(fmt.Errorf("%w: %s", err, out)) {
				kindrig.LogPhase(config.Cluster, deletion.name, "absent", started, "")
				continue
			}
			return fmt.Errorf("delete %s: %w: %s",
				deletion.name, err, strings.TrimSpace(string(out)))
		}
		kindrig.LogPhase(config.Cluster, deletion.name, "deleted", started, "")
	}
	return nil
}

// WriteKubeconfig fetches the configured cluster's credential into path and
// nothing else: KUBECONFIG is set for the child process only, so the
// operator's own kubeconfig is never touched and no ambient context can
// leak into a deploy (the GH-1341 discipline, applied to GKE).
func WriteKubeconfig(config Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create kubeconfig directory: %w", err)
	}
	cmd := exec.Command("gcloud", "container", "clusters", "get-credentials", config.Cluster,
		"--project", config.Project, "--region", config.Region)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("get-credentials %q: %w: %s",
			config.Cluster, err, strings.TrimSpace(string(out)))
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict kubeconfig %s: %w", path, err)
	}
	return nil
}
