// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package gcprig

import (
	"fmt"
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
	if err := EnsureBucket(run, config); err != nil {
		return err
	}
	if err := EnsureIdentity(run, config); err != nil {
		return err
	}
	if err := EnsureRegistry(run, config); err != nil {
		return err
	}
	kindrig.LogPhase(config.Cluster, "gcp-up", "complete", time.Now(),
		"bucket="+config.BucketURL()+" registry="+config.RegistryPath())
	return nil
}

// DownConfirmation is the exact argument gcp:down requires. A teardown in a
// shared project deletes real resources; the confirmation is typed, not
// defaulted.
const DownConfirmation = "delete-gcp-rig"

// Down deletes only the configured names, cluster last, behind the typed
// confirmation. A describe miss is a skip, not an error: deleting an absent
// resource is the state teardown wants.
func Down(run CommandRunner, config Config, confirmation string) error {
	if confirmation != DownConfirmation {
		return fmt.Errorf("gcp:down deletes cluster %q, bucket gs://%s, service account %s, "+
			"and registry %s in project %s; run again with the confirmation argument %q",
			config.Cluster, config.Bucket, config.GSAEmail(), config.Registry,
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
		{"service-account", []string{"gcloud", "iam", "service-accounts", "delete",
			config.GSAEmail(), "--project", config.Project, "--quiet"}},
		{"bucket", []string{"gcloud", "storage", "rm", "--recursive", "gs://" + config.Bucket}},
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
