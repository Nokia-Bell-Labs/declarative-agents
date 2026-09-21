// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package gcprig provisions the GCP demo rig: the cloud twin of kindrig
// (eng08-gcp-demo-rig). One Autopilot cluster, one GCS bucket for the
// objectstore family, the workload-identity pair that lets pods reach it
// with no stored credential, and the Artifact Registry the images push to.
// Everything is create-or-reuse by name, readiness is observed rather than
// assumed, and teardown removes only the configured names.
package gcprig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFile is the optional checked-in override beside the repository
// root: the demo.yaml pattern applied to the cloud (eng07). An absent file
// is the literal defaults; a malformed one is a named error, never a
// silent fallback.
const ConfigFile = "gcp.yaml"

// Config names every resource the rig touches. Teardown deletes exactly
// these names and nothing else.
type Config struct {
	// Project has no sensible default; preflight refuses an empty one by
	// name rather than guessing at someone's billing account.
	Project string `yaml:"project"`
	Region  string `yaml:"region"`
	Cluster string `yaml:"cluster"`
	// Bucket defaults to <project>-agents, since bucket names are global
	// and the project id is the one namespace the operator already owns.
	Bucket string `yaml:"bucket"`
	// Registry is the Artifact Registry repository name; images live under
	// <region>-docker.pkg.dev/<project>/<registry>/.
	Registry string `yaml:"registry"`
	// ServiceAccount is the GSA short name granted bucket access.
	ServiceAccount string `yaml:"service_account"`
	// Namespace and KSA name the Kubernetes side of the workload-identity
	// binding: the pods that may act as the GSA.
	Namespace string `yaml:"namespace"`
	KSA       string `yaml:"ksa"`
	// NodePlatform is the architecture the cluster's nodes run. It is not
	// the workstation's: an arm64 host that pushes its own build leaves
	// amd64 nodes reporting "no match for platform in manifest" (GH-2457).
	NodePlatform string `yaml:"node_platform"`
	// NodeServiceAccount is the identity the cluster's nodes run as, which
	// needs read on the registry to pull. Empty means the project's default
	// compute service account, which is what Autopilot uses; a cluster that
	// runs as something else names it here.
	NodeServiceAccount string `yaml:"node_service_account"`
}

// Defaults are the literal fallbacks (eng07). Project is empty on purpose.
func Defaults() Config {
	return Config{
		Region:         "us-central1",
		Cluster:        "da-gcp",
		Registry:       "agents",
		ServiceAccount: "agents-objectstore",
		// The namespace the chatbot-mesh release installs into. It was
		// da-chatbot-mesh-demo, which is the kind cluster's name and not a
		// namespace the mesh ever creates, so the binding was made for
		// pods that do not exist (GH-2458).
		Namespace:    "default",
		KSA:          "default",
		NodePlatform: "linux/amd64",
	}
}

// Load reads gcp.yaml beside root when present and overlays it on the
// defaults. A missing file is the defaults; a malformed file is an error
// naming the file.
func Load(root string) (Config, error) {
	config := Defaults()
	data, err := os.ReadFile(filepath.Join(root, ConfigFile))
	if err != nil {
		if os.IsNotExist(err) {
			return config.withDerived(), nil
		}
		return Config{}, fmt.Errorf("read %s: %w", ConfigFile, err)
	}
	var overrides Config
	if err := yaml.Unmarshal(data, &overrides); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", ConfigFile, err)
	}
	config.apply(overrides)
	return config.withDerived(), nil
}

func (c *Config) apply(overrides Config) {
	fields := []struct {
		target *string
		value  string
	}{
		{&c.Project, overrides.Project}, {&c.Region, overrides.Region},
		{&c.Cluster, overrides.Cluster}, {&c.Bucket, overrides.Bucket},
		{&c.Registry, overrides.Registry}, {&c.ServiceAccount, overrides.ServiceAccount},
		{&c.Namespace, overrides.Namespace}, {&c.KSA, overrides.KSA},
		{&c.NodeServiceAccount, overrides.NodeServiceAccount},
		{&c.NodePlatform, overrides.NodePlatform},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) != "" {
			*field.target = strings.TrimSpace(field.value)
		}
	}
}

func (c Config) withDerived() Config {
	if c.Bucket == "" && c.Project != "" {
		c.Bucket = c.Project + "-agents"
	}
	return c
}

// GSAEmail is the full service-account address the bindings grant.
func (c Config) GSAEmail() string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", c.ServiceAccount, c.Project)
}

// WorkloadIdentityMember is the principal the KSA acts as under workload
// identity.
func (c Config) WorkloadIdentityMember() string {
	return fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", c.Project, c.Namespace, c.KSA)
}

// KSAAnnotation is the annotation value the overlay places on the KSA so
// GKE maps it to the GSA.
func (c Config) KSAAnnotation() string {
	return c.GSAEmail()
}

// NodeServiceAccountEmail is the identity the nodes pull as. Autopilot runs
// nodes as the project's default compute service account, whose address is
// derived from the project number; an explicit override wins. The number is
// not derivable from the project id, so the caller passes what gcloud
// reported.
func (c Config) NodeServiceAccountEmail(projectNumber string) string {
	if strings.TrimSpace(c.NodeServiceAccount) != "" {
		return strings.TrimSpace(c.NodeServiceAccount)
	}
	return fmt.Sprintf("%s-compute@developer.gserviceaccount.com", strings.TrimSpace(projectNumber))
}

// RegistryPath is the image path prefix pushes and overlays use.
func (c Config) RegistryPath() string {
	return fmt.Sprintf("%s-docker.pkg.dev/%s/%s", c.Region, c.Project, c.Registry)
}

// BucketURL is the objectstore bucket URL the overlay hands the words
// (srd059 R2.3): no endpoint override, so ambient identity resolves it.
func (c Config) BucketURL() string {
	return "gs://" + c.Bucket
}
