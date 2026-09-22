// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validManifest() Manifest {
	m := Manifest{SchemaVersion: 1, Application: "chatbot-mesh", Ownership: "agent-owning"}
	m.Deployment.Entries = []DeploymentEntry{
		{ID: "chatbot", Workload: "chatbot", ProfilePath: "agents/chatbot/profile.yaml"},
		{ID: "collector", Workload: "collector", ProfilePath: "agents/collector/profile.yaml"},
	}
	return m
}

func binding() PlatformBinding {
	return PlatformBinding{
		Cluster: "da-platform", NamespacePrefix: "da-", IngressHostSuffix: "localhost",
		ChartName: "chatbot-mesh", ChartPath: "helm", ValuesPath: "helm/ci/kind-values.yaml",
		Timeout: "5m", ApplicationRoot: "/repo/applications/chatbot-mesh",
	}
}

// #2492 AC1: a conforming manifest validates; each structural fault is named.
func TestManifestValidation(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	cases := map[string]func(*Manifest){
		"not a DNS-1123 label":        func(m *Manifest) { m.Application = "Chatbot_Mesh" },
		"schema_version is required":  func(m *Manifest) { m.SchemaVersion = 0 },
		"deployment.entries is empty": func(m *Manifest) { m.Deployment.Entries = nil },
		"names no profile_path":       func(m *Manifest) { m.Deployment.Entries[0].ProfilePath = "" },
		"declared twice":              func(m *Manifest) { m.Deployment.Entries[1].Workload = "chatbot" },
	}
	for want, mutate := range cases {
		t.Run(want, func(t *testing.T) {
			m := validManifest()
			mutate(&m)
			err := m.Validate()
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("validate = %v, want a fault naming %q", err, want)
			}
		})
	}
}

func TestLoadManifestFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.yaml")
	body := "schema_version: 1\napplication: coding-agent\nownership: composition-only\n" +
		"deployment:\n  entries:\n    - {id: planner, workload: planner, profile_path: agents/planner/profile.yaml}\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil || m.Application != "coding-agent" || len(m.ProfileRoots()) != 1 {
		t.Fatalf("LoadManifest = %+v, %v", m, err)
	}
	if _, err := LoadManifest(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("LoadManifest accepted an absent path")
	}
}

// #2492 AC1: resolution derives complete coordinates, and a binding missing the
// cluster or chart is rejected by name.
func TestResolveDerivesCoordinates(t *testing.T) {
	r, err := Resolve(validManifest(), binding())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	checks := map[string]string{
		"namespace": r.Namespace, "release": r.Release, "host": r.Host,
		"bucket": r.BucketURL, "collector endpoint": r.CollectorEndpoint,
	}
	want := map[string]string{
		"namespace": "da-chatbot-mesh", "release": "chatbot-mesh",
		"host": "chatbot-mesh.localhost", "bucket": "gs://chatbot-mesh-telemetry",
		"collector endpoint": "chatbot-mesh-chatbot-mesh-collector.da-chatbot-mesh.svc:4317",
	}
	for name, got := range checks {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
	if err := r.Coordinates.Validate(); err != nil {
		// Coordinates.Validate also requires Kubeconfig/OverridesPath, which the
		// live path fills; assert the fields apprig owns are set.
		if r.Coordinates.Release == "" || r.Coordinates.Namespace == "" || r.Coordinates.ChartPath == "" {
			t.Errorf("resolved coordinates missing an apprig-owned field: %+v", r.Coordinates)
		}
	}
	if len(r.ProfileRoots) != 2 {
		t.Errorf("profile roots = %v, want the two deployment entries", r.ProfileRoots)
	}
}

func TestResolveRejectsIncompleteBinding(t *testing.T) {
	if _, err := Resolve(validManifest(), PlatformBinding{ChartPath: "helm"}); err == nil {
		t.Fatal("resolve accepted a binding with no cluster")
	}
	if _, err := Resolve(validManifest(), PlatformBinding{Cluster: "da-platform"}); err == nil {
		t.Fatal("resolve accepted a binding with no chart path")
	}
}

// #2492 AC2: two applications sharing a namespace, host, or bucket collide
// deterministically; distinct applications do not.
func TestDetectCollisions(t *testing.T) {
	a, _ := Resolve(validManifest(), binding())
	other := validManifest()
	other.Application = "coding-agent"
	otherBinding := binding()
	otherBinding.ChartName = "coding-agent"
	otherBinding.ApplicationRoot = "/repo/applications/coding-agent"
	b, _ := Resolve(other, otherBinding)
	if got := DetectCollisions([]Resolved{a, b}); len(got) != 0 {
		t.Fatalf("distinct applications collided: %+v", got)
	}

	// Two distinct applications misconfigured onto a shared namespace and
	// bucket -- the overwrite R11 exists to catch -- collide on exactly those
	// fields and name both applications.
	foo := Resolved{Application: "foo", Namespace: "da-shared", Release: "foo", Host: "foo.localhost", BucketURL: "gs://shared-telemetry", ObjectPrefix: "foo", Workspace: "/foo/deploy", EvidenceDir: "/foo/evidence", CollectorService: "foo-collector"}
	bar := Resolved{Application: "bar", Namespace: "da-shared", Release: "bar", Host: "bar.localhost", BucketURL: "gs://shared-telemetry", ObjectPrefix: "bar", Workspace: "/bar/deploy", EvidenceDir: "/bar/evidence", CollectorService: "bar-collector"}
	collisions := DetectCollisions([]Resolved{foo, bar})
	fields := map[string]bool{}
	for _, collision := range collisions {
		fields[collision.Field] = true
		if len(collision.Applications) != 2 || collision.Applications[0] != "bar" || collision.Applications[1] != "foo" {
			t.Errorf("collision %+v does not name both applications sorted", collision)
		}
	}
	if !fields["namespace"] || !fields["bucket"] {
		t.Errorf("expected namespace and bucket collisions, got %v", fields)
	}
	if fields["host"] || fields["release"] {
		t.Errorf("distinct host/release wrongly reported as collisions: %v", fields)
	}
}

// #2492 AC3: status is read-only and reports each surface separately; a nil
// probe is unknown, never invented.
func TestAggregateStatusIsReadOnlyAndSeparate(t *testing.T) {
	r, _ := Resolve(validManifest(), binding())
	report := AggregateStatus(r, StatusProbes{
		Bucket:        func() ComponentStatus { return ComponentStatus{State: StateOK, Detail: "gs://chatbot-mesh-telemetry"} },
		Collector:     func() ComponentStatus { return ComponentStatus{State: StateDegraded, Detail: "bucket unreachable"} },
		QueryEndpoint: func() ComponentStatus { return ComponentStatus{State: StateAbsent} },
	})
	if report.Bucket.State != StateOK {
		t.Errorf("bucket = %+v, want ok", report.Bucket)
	}
	// A degraded collector must not drag the bucket down: surfaces are separate.
	if report.Collector.State != StateDegraded || report.Bucket.State != StateOK {
		t.Errorf("degraded collector conflated with bucket: %+v / %+v", report.Collector, report.Bucket)
	}
	if report.QueryEndpoint.State != StateAbsent {
		t.Errorf("query endpoint = %+v, want absent", report.QueryEndpoint)
	}
	// Unwired probes are unknown, not assumed healthy.
	if report.Platform.State != StateUnknown || report.WAL.State != StateUnknown {
		t.Errorf("nil probes were not reported unknown: %+v", report)
	}
}

// #2492 AC4, #2479 R13/AC11: apprig contains no direct Helm lifecycle argv and
// no second diagnosis or purge state machine -- those run through the canonical
// agents via kindrig.
func TestNoDirectHelmLifecycleOrSecondStateMachine(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		`"helm"`, "helm upgrade", "helm install", "helm uninstall",
		"exec.Command(\"helm\"", "exec.Command(\"kubectl\"",
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, token := range forbidden {
			if strings.Contains(source, token) {
				t.Errorf("%s contains %q; lifecycle argv must run through kindrig's canonical agents, not apprig", name, token)
			}
		}
	}
}
