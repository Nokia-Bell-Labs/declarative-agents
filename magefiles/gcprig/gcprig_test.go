// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package gcprig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recorder captures every command and answers from a script keyed on a
// command prefix, mirroring the kindrig recording-runner tests: the suite
// proves the sequences without gcloud, a network, or a project.
type recorder struct {
	calls   []string
	answers map[string]answer
}

type answer struct {
	out string
	err error
}

func (r *recorder) run(name string, args ...string) ([]byte, error) {
	command := strings.Join(append([]string{name}, args...), " ")
	r.calls = append(r.calls, command)
	for prefix, response := range r.answers {
		if strings.HasPrefix(command, prefix) {
			return []byte(response.out), response.err
		}
	}
	return nil, nil
}

func (r *recorder) sequence() string { return strings.Join(r.calls, "\n") }

func testConfig() Config {
	config := Defaults()
	config.Project = "demo-project"
	return config.withDerived()
}

func withGcloudPresent(t *testing.T) {
	t.Helper()
	previous := LookPath
	LookPath = func(string) (string, error) { return "/usr/bin/gcloud", nil }
	t.Cleanup(func() { LookPath = previous })
}

func withoutSleeping(t *testing.T) {
	t.Helper()
	previous := Sleep
	Sleep = func(d time.Duration) {}
	t.Cleanup(func() { Sleep = previous })
}

// The defaults are literal, gcp.yaml overlays them, an absent file is the
// defaults, and a malformed file is a named error (eng07, eng08).
func TestConfigDefaultsAndOverride(t *testing.T) {
	root := t.TempDir()
	config, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Region != "us-central1" || config.Cluster != "da-gcp" {
		t.Fatalf("defaults = %+v", config)
	}
	if config.Project != "" || config.Bucket != "" {
		t.Fatalf("project must default empty, got %+v", config)
	}

	if err := os.WriteFile(filepath.Join(root, ConfigFile),
		[]byte("project: demo-project\ncluster: my-cluster\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Project != "demo-project" || config.Cluster != "my-cluster" {
		t.Fatalf("override = %+v", config)
	}
	if config.Bucket != "demo-project-agents" {
		t.Fatalf("derived bucket = %q", config.Bucket)
	}
	if config.Region != "us-central1" {
		t.Fatalf("unset field lost its default: %+v", config)
	}

	if err := os.WriteFile(filepath.Join(root, ConfigFile),
		[]byte(":\tnot yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), ConfigFile) {
		t.Fatalf("malformed config error = %v", err)
	}
}

func TestDerivedIdentityValues(t *testing.T) {
	config := testConfig()
	if got := config.GSAEmail(); got != "agents-objectstore@demo-project.iam.gserviceaccount.com" {
		t.Errorf("GSA = %q", got)
	}
	if got := config.WorkloadIdentityMember(); got != "serviceAccount:demo-project.svc.id.goog[da-chatbot-mesh-demo/default]" {
		t.Errorf("member = %q", got)
	}
	if got := config.RegistryPath(); got != "us-central1-docker.pkg.dev/demo-project/agents" {
		t.Errorf("registry = %q", got)
	}
	if got := config.BucketURL(); got != "gs://demo-project-agents" {
		t.Errorf("bucket URL = %q", got)
	}
}

// Every preflight failure names its remedy (eng08: fail, never skip).
func TestPreflightNamesEachRemedy(t *testing.T) {
	previous := LookPath
	LookPath = func(string) (string, error) { return "", errors.New("absent") }
	t.Cleanup(func() { LookPath = previous })
	err := Preflight(nil, testConfig())
	if err == nil || !strings.Contains(err.Error(), "cloud.google.com/sdk") {
		t.Fatalf("missing gcloud: %v", err)
	}

	withGcloudPresent(t)
	config := testConfig()
	config.Project = ""
	if err := Preflight(nil, config); err == nil || !strings.Contains(err.Error(), ConfigFile) {
		t.Fatalf("missing project: %v", err)
	}

	unauthenticated := &recorder{answers: map[string]answer{
		"gcloud auth list": {out: "\n"},
	}}
	if err := Preflight(unauthenticated.run, testConfig()); err == nil ||
		!strings.Contains(err.Error(), "gcloud auth login") {
		t.Fatalf("unauthenticated: %v", err)
	}

	noProject := &recorder{answers: map[string]answer{
		"gcloud auth list":         {out: "person@example.com\n"},
		"gcloud projects describe": {out: "PERMISSION_DENIED", err: errors.New("exit 1")},
	}}
	if err := Preflight(noProject.run, testConfig()); err == nil ||
		!strings.Contains(err.Error(), "demo-project") {
		t.Fatalf("inaccessible project: %v", err)
	}
}

// A RUNNING cluster is reused with zero mutating calls; any other state is
// refused; an absent one is created and readiness observed.
func TestEnsureClusterReuseRefuseCreate(t *testing.T) {
	withoutSleeping(t)
	running := &recorder{answers: map[string]answer{
		"gcloud container clusters describe": {out: "RUNNING\n"},
	}}
	if err := EnsureCluster(running.run, testConfig()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(running.sequence(), "create-auto") {
		t.Fatalf("reuse issued a create:\n%s", running.sequence())
	}
	if !strings.Contains(running.sequence(), "get-credentials") {
		t.Fatalf("reuse skipped get-credentials:\n%s", running.sequence())
	}

	degraded := &recorder{answers: map[string]answer{
		"gcloud container clusters describe": {out: "DEGRADED\n"},
	}}
	if err := EnsureCluster(degraded.run, testConfig()); err == nil ||
		!strings.Contains(err.Error(), "refusing to adopt") {
		t.Fatalf("degraded cluster: %v", err)
	}

	var describes int
	creating := &recorder{}
	creating.answers = map[string]answer{}
	create := func(name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		creating.calls = append(creating.calls, command)
		if strings.Contains(command, "clusters describe") {
			describes++
			switch {
			case describes == 1:
				return []byte("NOT_FOUND"), errors.New("NOT_FOUND")
			case describes < 4:
				return []byte("PROVISIONING\n"), nil
			default:
				return []byte("RUNNING\n"), nil
			}
		}
		return nil, nil
	}
	if err := EnsureCluster(create, testConfig()); err != nil {
		t.Fatal(err)
	}
	sequence := creating.sequence()
	if !strings.Contains(sequence, "create-auto da-gcp") {
		t.Fatalf("no create in:\n%s", sequence)
	}
	if describes < 4 {
		t.Fatalf("readiness was not observed: %d describes", describes)
	}
	if !strings.Contains(sequence, "get-credentials") {
		t.Fatalf("no get-credentials in:\n%s", sequence)
	}
}

// A cluster that reports ERROR during creation fails by name instead of
// polling to the deadline.
func TestEnsureClusterFailsOnError(t *testing.T) {
	withoutSleeping(t)
	var describes int
	run := func(name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		if strings.Contains(command, "clusters describe") {
			describes++
			if describes == 1 {
				return []byte("NOT_FOUND"), errors.New("NOT_FOUND")
			}
			return []byte("ERROR\n"), nil
		}
		return nil, nil
	}
	if err := EnsureCluster(run, testConfig()); err == nil ||
		!strings.Contains(err.Error(), "ERROR") {
		t.Fatalf("error state: %v", err)
	}
}

// Bucket, identity, and registry: reuse issues no create, absence creates,
// and the identity bindings carry the exact member and role strings.
func TestEnsureBucketIdentityRegistrySequences(t *testing.T) {
	reused := &recorder{answers: map[string]answer{
		"gcloud storage buckets describe":        {out: "demo-project-agents\n"},
		"gcloud iam service-accounts describe":   {out: "x@y\n"},
		"gcloud artifacts repositories describe": {out: "agents\n"},
	}}
	config := testConfig()
	if err := EnsureBucket(reused.run, config); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRegistry(reused.run, config); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reused.sequence(), " create") {
		t.Fatalf("reuse issued a create:\n%s", reused.sequence())
	}

	absent := &recorder{answers: map[string]answer{
		"gcloud storage buckets describe": {
			out: "ERROR: (gcloud.storage.buckets.describe) gs://demo-project-agents not found: 404.",
			err: errors.New("exit status 1")},
		"gcloud iam service-accounts describe": {
			out: "ERROR: (gcloud.iam.service-accounts.describe) NOT_FOUND",
			err: errors.New("exit status 1")},
		"gcloud artifacts repositories describe": {
			out: "ERROR: NOT_FOUND: Requested entity was not found",
			err: errors.New("exit status 1")},
	}}
	if err := EnsureBucket(absent.run, config); err != nil {
		t.Fatal(err)
	}
	if err := EnsureIdentity(absent.run, config); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRegistry(absent.run, config); err != nil {
		t.Fatal(err)
	}
	sequence := absent.sequence()
	for _, want := range []string{
		"gcloud storage buckets create gs://demo-project-agents",
		"--uniform-bucket-level-access",
		"gcloud iam service-accounts create agents-objectstore",
		"gcloud storage buckets add-iam-policy-binding gs://demo-project-agents --member serviceAccount:agents-objectstore@demo-project.iam.gserviceaccount.com --role roles/storage.objectAdmin",
		"--member serviceAccount:demo-project.svc.id.goog[da-chatbot-mesh-demo/default] --role roles/iam.workloadIdentityUser",
		"gcloud artifacts repositories create agents",
		"--repository-format docker",
	} {
		if !strings.Contains(sequence, want) {
			t.Errorf("sequence missing %q:\n%s", want, sequence)
		}
	}
}

// Up composes preflight through registry; a preflight failure stops before
// any resource call.
func TestUpStopsAtPreflight(t *testing.T) {
	withGcloudPresent(t)
	rec := &recorder{answers: map[string]answer{
		"gcloud auth list": {out: "\n"},
	}}
	if err := Up(rec.run, testConfig()); err == nil ||
		!strings.Contains(err.Error(), "gcloud auth login") {
		t.Fatalf("preflight: %v", err)
	}
	for _, call := range rec.calls {
		if strings.Contains(call, "clusters") || strings.Contains(call, "buckets") {
			t.Fatalf("resource call before preflight passed: %s", call)
		}
	}
}

// Down refuses without the typed confirmation, deletes only the configured
// names in reverse order with the cluster last, and treats absence as done.
func TestDownConfirmationScopeAndOrder(t *testing.T) {
	withGcloudPresent(t)
	config := testConfig()
	if err := Down(nil, config, "yes"); err == nil ||
		!strings.Contains(err.Error(), DownConfirmation) {
		t.Fatalf("unconfirmed down: %v", err)
	}

	rec := &recorder{answers: map[string]answer{
		"gcloud auth list":                   {out: "person@example.com\n"},
		"gcloud projects describe":           {out: "demo-project\n"},
		"gcloud iam service-accounts delete": {out: "NOT_FOUND", err: errors.New("NOT_FOUND")},
	}}
	if err := Down(rec.run, config, DownConfirmation); err != nil {
		t.Fatal(err)
	}
	sequence := rec.sequence()
	for _, want := range []string{
		"repositories delete agents",
		"gcloud storage rm --recursive gs://demo-project-agents",
		"clusters delete da-gcp",
	} {
		if !strings.Contains(sequence, want) {
			t.Errorf("down missing %q:\n%s", want, sequence)
		}
	}
	if !strings.Contains(sequence, "--quiet") {
		t.Errorf("down without --quiet would prompt interactively:\n%s", sequence)
	}
	registryIndex := strings.Index(sequence, "repositories delete")
	clusterIndex := strings.Index(sequence, "clusters delete")
	if clusterIndex < registryIndex {
		t.Errorf("cluster deleted before the resources that live on it:\n%s", sequence)
	}
}

// The push refuses latest and an empty revision by name (eng01, eng08).
func TestPushAgentCoreRefusesFloatingTags(t *testing.T) {
	config := testConfig()
	if _, err := PushAgentCore(nil, config, "agent-core:local", ""); err == nil {
		t.Fatal("empty revision accepted")
	}
	if _, err := PushAgentCore(nil, config, "agent-core:local", "latest"); err == nil ||
		!strings.Contains(err.Error(), "eng01") {
		t.Fatalf("latest: %v", err)
	}
	rec := &recorder{answers: map[string]answer{}}
	target, err := PushAgentCore(rec.run, config, "agent-core:local", "a1b2c3d4e5f6")
	if err != nil {
		t.Fatal(err)
	}
	if target != "us-central1-docker.pkg.dev/demo-project/agents/agent-core:a1b2c3d4e5f6" {
		t.Fatalf("target = %q", target)
	}
	if !strings.Contains(rec.sequence(), "docker push "+target) {
		t.Fatalf("no push in:\n%s", rec.sequence())
	}
}

// The donor mirror pulls the pinned source, pushes under the registry path,
// and reports the mirror digest for the overlay pin.
// The mirror copies the manifest list rather than pulling: a docker pull on
// one architecture uploads a single-platform image to a registry serving
// another, which is what GH-2437 caught on a real project.
func TestMirrorDonorCopiesTheIndexAndReportsItsDigest(t *testing.T) {
	listing := "Name:      us-central1-docker.pkg.dev/demo-project/agents/cli-donor:1.31.4\n" +
		"MediaType: application/vnd.docker.distribution.manifest.list.v2+json\n" +
		"Digest:    sha256:9c4976d4\n\nManifests:\n" +
		"  Name:      …@sha256:c4e12eb3\n  Platform:  linux/amd64\n" +
		"  Name:      …@sha256:0f0f4f1c\n  Platform:  linux/arm64\n"
	rec := &recorder{answers: map[string]answer{
		"docker buildx imagetools inspect": {out: listing},
	}}
	reference, err := MirrorDonor(rec.run, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	sequence := rec.sequence()
	if !strings.Contains(sequence, "docker buildx imagetools create --tag "+
		"us-central1-docker.pkg.dev/demo-project/agents/cli-donor:1.31.4 docker.io/alpine/k8s:1.31.4@sha256:") {
		t.Fatalf("mirror does not copy the index:\n%s", sequence)
	}
	if strings.Contains(sequence, "docker pull") || strings.Contains(sequence, "docker push") {
		t.Fatalf("mirror pulls or pushes a single platform:\n%s", sequence)
	}
	want := "us-central1-docker.pkg.dev/demo-project/agents/cli-donor:1.31.4@sha256:9c4976d4"
	if reference != want {
		t.Fatalf("reference = %q, want %q", reference, want)
	}
}

// The top-level digest is the manifest list's, never a platform's.
func TestMirrorDigestReadsTheListDigest(t *testing.T) {
	listing := "Name: x\nMediaType: y\nDigest:    sha256:list\n\nManifests:\n" +
		"  Name: x@sha256:platform\n  Digest: sha256:platform\n"
	if got := mirrorDigest(listing); got != "sha256:list" {
		t.Fatalf("mirrorDigest = %q, want the list digest", got)
	}
	if got := mirrorDigest("Name: x\nno digest here\n"); got != "" {
		t.Fatalf("mirrorDigest = %q, want empty", got)
	}
}
