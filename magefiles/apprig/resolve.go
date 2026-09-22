// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// PlatformBinding is the platform-side input the runner resolves an
// application against: where the platform is, and the conventions that turn an
// application name into a namespace, host, bucket, and evidence coordinates. It
// is declared configuration, not ambient context (#2479 R3, R9).
type PlatformBinding struct {
	// Cluster is the platform cluster the release targets (da-platform).
	Cluster string
	// KubeconfigPath, when set, points the deploy at a specific cluster
	// (the GKE path); empty asks kind for the platform kubeconfig.
	KubeconfigPath string
	// NamespacePrefix is prepended to the application name (da-).
	NamespacePrefix string
	// IngressHostSuffix is joined to the application name for the external
	// host (chatbot-mesh.<suffix>).
	IngressHostSuffix string
	// BucketURLTemplate is a single %s printf taking the application name;
	// empty defaults to gs://%s-telemetry. It never carries credentials.
	BucketURLTemplate string
	// ChartName is the agent-services fullname base the collector Service is
	// named from (<release>-<chartName>-collector).
	ChartName string
	// ChartPath and ValuesPath are the application's chart and checked-in
	// overlay the deploy words install.
	ChartPath  string
	ValuesPath string
	// Timeout bounds the apply and rollout waits.
	Timeout string
	// ApplicationRoot is where build/deploy and build/kind-evidence live.
	ApplicationRoot string
	// CollectorOTLPPort is the in-release collector's OTLP gRPC port.
	CollectorOTLPPort int
}

// Resolved is every coordinate the lifecycle verbs need, derived once from the
// manifest and the platform binding. The live sibling passes Coordinates to
// kindrig.Deploy/Undeploy and the rest to status and evidence (#2479 R3).
type Resolved struct {
	Application       string
	Namespace         string
	Release           string
	Host              string
	BucketURL         string
	ObjectPrefix      string
	WALPath           string
	Workspace         string
	EvidenceDir       string
	CollectorService  string
	CollectorEndpoint string
	ProfileRoots      []string
	Coordinates       kindrig.DeployCoordinates
}

const (
	defaultNamespacePrefix = "da-"
	defaultBucketTemplate  = "gs://%s-telemetry"
	defaultCollectorPort   = 4317
	defaultWALMount        = "/data/wal"
)

// Resolve derives one application's coordinates. It fills defaults for the
// optional binding fields and rejects a binding missing the chart or values a
// deploy cannot proceed without, so a fault surfaces here rather than as a
// deploy against the wrong cluster (#2479 R3).
func Resolve(manifest Manifest, binding PlatformBinding) (Resolved, error) {
	if strings.TrimSpace(binding.Cluster) == "" {
		return Resolved{}, fmt.Errorf("resolve %s: platform binding names no cluster", manifest.Application)
	}
	if binding.ChartPath == "" {
		return Resolved{}, fmt.Errorf("resolve %s: platform binding names no chart path", manifest.Application)
	}
	app := manifest.Application
	prefix := binding.NamespacePrefix
	if prefix == "" {
		prefix = defaultNamespacePrefix
	}
	namespace := prefix + app
	if !dnsLabel.MatchString(namespace) {
		return Resolved{}, fmt.Errorf("resolve %s: namespace %q is not a DNS-1123 label", app, namespace)
	}
	bucketTemplate := binding.BucketURLTemplate
	if bucketTemplate == "" {
		bucketTemplate = defaultBucketTemplate
	}
	chartName := binding.ChartName
	if chartName == "" {
		chartName = app
	}
	port := binding.CollectorOTLPPort
	if port == 0 {
		port = defaultCollectorPort
	}
	root := binding.ApplicationRoot
	collectorService := fmt.Sprintf("%s-%s-collector", app, chartName)
	resolved := Resolved{
		Application:       app,
		Namespace:         namespace,
		Release:           app,
		Host:              hostFor(app, binding.IngressHostSuffix),
		BucketURL:         fmt.Sprintf(bucketTemplate, app),
		ObjectPrefix:      app,
		WALPath:           defaultWALMount,
		Workspace:         filepath.Join(root, "build", "deploy", app),
		EvidenceDir:       filepath.Join(root, "build", "kind-evidence"),
		CollectorService:  collectorService,
		CollectorEndpoint: fmt.Sprintf("%s.%s.svc:%d", collectorService, namespace, port),
		ProfileRoots:      manifest.ProfileRoots(),
		Coordinates: kindrig.DeployCoordinates{
			Release:    app,
			Namespace:  namespace,
			ChartPath:  binding.ChartPath,
			ValuesPath: binding.ValuesPath,
			Timeout:    binding.Timeout,
		},
	}
	return resolved, nil
}

func hostFor(app, suffix string) string {
	if suffix == "" {
		return app
	}
	return app + "." + suffix
}

// Collision is one field two or more applications resolved to the same value.
// Sharing any of these on one platform lets one application overwrite
// another's release, data, or evidence (#2479 R11).
type Collision struct {
	Field        string
	Value        string
	Applications []string
}

// DetectCollisions reports every field on which two or more resolved
// applications share a value. It is deterministic: the collisions and their
// application lists are sorted, so the same input always reports the same
// findings (#2479 R11, AC7).
func DetectCollisions(resolved []Resolved) []Collision {
	fields := []struct {
		name  string
		value func(Resolved) string
	}{
		{"namespace", func(r Resolved) string { return r.Namespace }},
		{"release", func(r Resolved) string { return r.Release }},
		{"host", func(r Resolved) string { return r.Host }},
		{"bucket", func(r Resolved) string { return r.BucketURL }},
		{"object_prefix", func(r Resolved) string { return r.ObjectPrefix }},
		{"workspace", func(r Resolved) string { return r.Workspace }},
		{"evidence_dir", func(r Resolved) string { return r.EvidenceDir }},
		{"collector_service", func(r Resolved) string { return r.CollectorService }},
	}
	var collisions []Collision
	for _, field := range fields {
		byValue := map[string][]string{}
		for _, item := range resolved {
			value := field.value(item)
			byValue[value] = append(byValue[value], item.Application)
		}
		for value, apps := range byValue {
			if len(dedupeSorted(apps)) < 2 {
				continue
			}
			collisions = append(collisions, Collision{
				Field: field.name, Value: value, Applications: dedupeSorted(apps),
			})
		}
	}
	sort.Slice(collisions, func(i, j int) bool {
		if collisions[i].Field != collisions[j].Field {
			return collisions[i].Field < collisions[j].Field
		}
		return collisions[i].Value < collisions[j].Value
	})
	return collisions
}

func dedupeSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
