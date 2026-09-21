// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package gcprig

import (
	"fmt"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// clusterReadyDeadline bounds how long Up waits for an Autopilot cluster to
// report RUNNING. Autopilot creation routinely takes minutes; readiness is
// observed by polling describe, never assumed from create returning.
const clusterReadyDeadline = 20 * time.Minute

// clusterPollInterval is how often the poll asks; the sleep is between
// observations, not in place of one.
const clusterPollInterval = 15 * time.Second

// serviceAccountReadyDeadline and its poll interval bound the wait for a
// newly created service account to become visible to the services that
// validate a binding's member. IAM answers the create before it has
// propagated, and a bind one second later is rejected as naming an account
// that does not exist (GH-2462).
const serviceAccountPollInterval = 3 * time.Second

// serviceAccountReadyDeadline is a var for the same reason Sleep is: a proof
// of the timeout must not wait two minutes to observe it.
var serviceAccountReadyDeadline = 2 * time.Minute

// Sleep is swapped in tests so the readiness poll is provable without
// waiting.
var Sleep = time.Sleep

// EnsureCluster creates or reuses the configured Autopilot cluster. An
// existing RUNNING cluster is reused; an existing cluster in any other
// state is refused, because its ownership is unknown (the kindrig add-on
// guard). On success the kubeconfig gains the cluster's context.
func EnsureCluster(run CommandRunner, config Config) error {
	status, err := clusterStatus(run, config)
	switch {
	case err == nil && status == "RUNNING":
		kindrig.LogPhase(config.Cluster, "cluster", "reused", time.Now(), "status=RUNNING")
	case err == nil:
		return fmt.Errorf("cluster %q already exists but is %q, not RUNNING; "+
			"refusing to adopt it", config.Cluster, status)
	case notFound(err):
		started := time.Now()
		if out, err := run("gcloud", "container", "clusters", "create-auto", config.Cluster,
			"--project", config.Project, "--region", config.Region,
			"--quiet"); err != nil {
			return fmt.Errorf("create cluster %q: %w: %s",
				config.Cluster, err, strings.TrimSpace(string(out)))
		}
		if err := waitClusterRunning(run, config); err != nil {
			return err
		}
		kindrig.LogPhase(config.Cluster, "cluster", "created", started, "status=RUNNING")
	default:
		return fmt.Errorf("describe cluster %q: %w", config.Cluster, err)
	}
	if out, err := run("gcloud", "container", "clusters", "get-credentials", config.Cluster,
		"--project", config.Project, "--region", config.Region); err != nil {
		return fmt.Errorf("get-credentials %q: %w: %s",
			config.Cluster, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func clusterStatus(run CommandRunner, config Config) (string, error) {
	out, err := run("gcloud", "container", "clusters", "describe", config.Cluster,
		"--project", config.Project, "--region", config.Region,
		"--format=value(status)")
	status := strings.TrimSpace(string(out))
	if err != nil {
		return status, fmt.Errorf("%w: %s", err, status)
	}
	return status, nil
}

func waitClusterRunning(run CommandRunner, config Config) error {
	deadline := time.Now().Add(clusterReadyDeadline)
	for {
		status, err := clusterStatus(run, config)
		if err == nil && status == "RUNNING" {
			return nil
		}
		if err == nil && status == "ERROR" {
			return fmt.Errorf("cluster %q entered ERROR during creation", config.Cluster)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("cluster %q did not reach RUNNING within %s (last status %q)",
				config.Cluster, clusterReadyDeadline, status)
		}
		Sleep(clusterPollInterval)
	}
}

// EnsureBucket creates or reuses the configured bucket with uniform
// bucket-level access.
func EnsureBucket(run CommandRunner, config Config) error {
	if out, err := run("gcloud", "storage", "buckets", "describe",
		"gs://"+config.Bucket, "--project", config.Project,
		"--format=value(name)"); err == nil {
		kindrig.LogPhase(config.Cluster, "bucket", "reused", time.Now(), "gs://"+config.Bucket)
		return nil
	} else if described := describeError(err, out); !notFound(described) {
		return fmt.Errorf("describe bucket gs://%s: %w", config.Bucket, described)
	}
	started := time.Now()
	if out, err := run("gcloud", "storage", "buckets", "create", "gs://"+config.Bucket,
		"--project", config.Project, "--location", config.Region,
		"--uniform-bucket-level-access"); err != nil {
		return fmt.Errorf("create bucket gs://%s: %w: %s",
			config.Bucket, err, strings.TrimSpace(string(out)))
	}
	kindrig.LogPhase(config.Cluster, "bucket", "created", started, "gs://"+config.Bucket)
	return nil
}

// EnsureIdentity creates or reuses the GSA, grants it objectAdmin scoped to
// the bucket, and binds the configured namespace/KSA pair as its workload
// identity user. Pods then reach the bucket with no stored credential
// (srd059 R2.2, eng08).
func EnsureIdentity(run CommandRunner, config Config) error {
	if out, err := run("gcloud", "iam", "service-accounts", "describe", config.GSAEmail(),
		"--project", config.Project, "--format=value(email)"); err == nil {
		kindrig.LogPhase(config.Cluster, "service-account", "reused", time.Now(), config.GSAEmail())
	} else if described := describeError(err, out); !serviceAccountAbsent(described) {
		return fmt.Errorf("describe service account %s: %w", config.GSAEmail(), described)
	} else {
		started := time.Now()
		if out, err := run("gcloud", "iam", "service-accounts", "create", config.ServiceAccount,
			"--project", config.Project,
			"--display-name", "declarative-agents objectstore"); err != nil {
			return fmt.Errorf("create service account %s: %w: %s",
				config.ServiceAccount, err, strings.TrimSpace(string(out)))
		}
		if err := waitServiceAccountVisible(run, config); err != nil {
			return err
		}
		kindrig.LogPhase(config.Cluster, "service-account", "created", started, config.GSAEmail())
	}
	// Both bindings are idempotent add-iam-policy-binding calls; gcloud
	// deduplicates an existing member/role pair, so reuse needs no describe.
	if out, err := run("gcloud", "storage", "buckets", "add-iam-policy-binding",
		"gs://"+config.Bucket,
		"--member", "serviceAccount:"+config.GSAEmail(),
		"--role", "roles/storage.objectAdmin"); err != nil {
		return fmt.Errorf("grant objectAdmin on gs://%s: %w: %s",
			config.Bucket, err, strings.TrimSpace(string(out)))
	}
	if out, err := run("gcloud", "iam", "service-accounts", "add-iam-policy-binding",
		config.GSAEmail(), "--project", config.Project,
		"--member", config.WorkloadIdentityMember(),
		"--role", "roles/iam.workloadIdentityUser"); err != nil {
		return fmt.Errorf("bind workload identity for %s: %w: %s",
			config.WorkloadIdentityMember(), err, strings.TrimSpace(string(out)))
	}
	kindrig.LogPhase(config.Cluster, "identity", "bound", time.Now(),
		"ksa-annotation="+config.KSAAnnotation())
	return nil
}

// waitServiceAccountVisible polls until the created account answers its own
// describe. Every other phase observes readiness rather than assuming it
// (eng08); this one did not, and a project that had never held the account
// failed one second after creating it, binding a member IAM did not yet
// know. A reused account never reaches this, because it was not created.
func waitServiceAccountVisible(run CommandRunner, config Config) error {
	deadline := time.Now().Add(serviceAccountReadyDeadline)
	for {
		if _, err := run("gcloud", "iam", "service-accounts", "describe", config.GSAEmail(),
			"--project", config.Project, "--format=value(email)"); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service account %s was created but did not become visible within %s",
				config.GSAEmail(), serviceAccountReadyDeadline)
		}
		Sleep(serviceAccountPollInterval)
	}
}

// EnsureRegistry creates or reuses the Artifact Registry repository.
func EnsureRegistry(run CommandRunner, config Config) error {
	if out, err := run("gcloud", "artifacts", "repositories", "describe", config.Registry,
		"--project", config.Project, "--location", config.Region,
		"--format=value(name)"); err == nil {
		kindrig.LogPhase(config.Cluster, "registry", "reused", time.Now(), config.RegistryPath())
		return nil
	} else if described := describeError(err, out); !notFound(described) {
		return fmt.Errorf("describe registry %s: %w", config.Registry, described)
	}
	started := time.Now()
	if out, err := run("gcloud", "artifacts", "repositories", "create", config.Registry,
		"--project", config.Project, "--location", config.Region,
		"--repository-format", "docker",
		"--description", "declarative-agents demo rig images"); err != nil {
		return fmt.Errorf("create registry %s: %w: %s",
			config.Registry, err, strings.TrimSpace(string(out)))
	}
	kindrig.LogPhase(config.Cluster, "registry", "created", started, config.RegistryPath())
	return nil
}

// EnsureRegistryAccess grants the cluster's node service account read on the
// registry. Creating the repository is not enough: a node with no read pulls
// nothing, and every pod reports ImagePullBackOff behind a 403 from the
// registry's token endpoint while the release's helm wait times out with the
// mesh half-up (GH-2456). The binding is idempotent, like the two in
// EnsureIdentity, so a reused project mutates nothing.
func EnsureRegistryAccess(run CommandRunner, config Config) error {
	member, err := nodeServiceAccountMember(run, config)
	if err != nil {
		return err
	}
	if out, err := run("gcloud", "artifacts", "repositories", "add-iam-policy-binding",
		config.Registry, "--project", config.Project, "--location", config.Region,
		"--member", member, "--role", "roles/artifactregistry.reader"); err != nil {
		return fmt.Errorf("grant registry read to %s: %w: %s",
			member, err, strings.TrimSpace(string(out)))
	}
	kindrig.LogPhase(config.Cluster, "registry-access", "bound", time.Now(), member)
	return nil
}

// nodeServiceAccountMember resolves the principal to grant. A configured
// account is taken as written; otherwise the project number is asked for,
// because the default compute account's address carries the number.
func nodeServiceAccountMember(run CommandRunner, config Config) (string, error) {
	if strings.TrimSpace(config.NodeServiceAccount) != "" {
		return "serviceAccount:" + config.NodeServiceAccountEmail(""), nil
	}
	number, err := projectNumber(run, config)
	if err != nil {
		return "", err
	}
	return "serviceAccount:" + config.NodeServiceAccountEmail(number), nil
}

// projectNumber asks gcloud for the number behind the configured project id.
// The default compute service account's address carries the number, not the
// id, and the two are unrelated strings.
func projectNumber(run CommandRunner, config Config) (string, error) {
	out, err := run("gcloud", "projects", "describe", config.Project,
		"--format=value(projectNumber)")
	if err != nil {
		return "", fmt.Errorf("describe project %s: %w", config.Project, describeError(err, out))
	}
	number := strings.TrimSpace(string(out))
	if number == "" {
		return "", fmt.Errorf("describe project %s: no project number in the answer", config.Project)
	}
	return number, nil
}

// describeError folds a command's combined output into its error. gcloud
// writes what happened to the output and exits 1, so the Go error alone is
// "exit status 1" and classification without the output reads nothing —
// the defect a real project surfaced in GH-2435.
func describeError(err error, out []byte) error {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, trimmed)
}

// serviceAccountAbsent reports whether an IAM describe means the account is
// not there to reuse. IAM does not distinguish absence from denial on a read
// and answers both with PERMISSION_DENIED — gcloud's own service-accounts
// undelete help states it: "If the service account does not exist, this
// command returns a PERMISSION_DENIED error." Treating that as fatal left
// gcp:up unable to create the account on a project that had never held it
// (GH-2455). Proceeding to the create is safe: a caller who genuinely lacks
// the permission fails there, and that error names the permission, which the
// describe's does not. Buckets and repositories keep the NOT_FOUND rule,
// because their APIs report absence.
func serviceAccountAbsent(err error) bool {
	if err == nil {
		return false
	}
	return notFound(err) ||
		strings.Contains(strings.ToUpper(err.Error()), "PERMISSION_DENIED")
}

// notFound reports whether a gcloud error names an absent resource. gcloud
// prints NOT_FOUND or 404 for describes of resources that do not exist;
// gcloud storage spells it lowercase with a trailing period.
func notFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	return strings.Contains(message, "NOT_FOUND") || strings.Contains(message, "404") ||
		strings.Contains(message, "NOT FOUND")
}
