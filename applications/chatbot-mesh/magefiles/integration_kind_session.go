// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

type integrationKindSession struct {
	mu         sync.Mutex
	root       string
	cluster    kindrig.Cluster
	kindRun    kindrig.Runner
	evidence   kindrig.FailureEvidence
	hostImages map[string]string
	finalizers map[string]func() error
	batching   bool
	poisoned   error
	closed     bool
}

var integrationKindSessionState struct {
	sync.Mutex
	active *integrationKindSession
}

func newIntegrationKindSession(root string) *integrationKindSession {
	return &integrationKindSession{
		root:    root,
		kindRun: kindrig.DefaultRun,
		evidence: kindrig.FailureEvidence{
			Directory:  filepath.Join(root, "build", "kind-evidence", kindrig.PlatformClusterName),
			Namespaces: []string{"default"},
		},
		hostImages: make(map[string]string),
		finalizers: make(map[string]func() error),
	}
}

func activateIntegrationKindSession(session *integrationKindSession) (func(), error) {
	integrationKindSessionState.Lock()
	defer integrationKindSessionState.Unlock()
	if integrationKindSessionState.active != nil {
		return nil, errors.New("chatbot integration kind session is already active")
	}
	integrationKindSessionState.active = session
	return func() {
		integrationKindSessionState.Lock()
		if integrationKindSessionState.active == session {
			integrationKindSessionState.active = nil
		}
		integrationKindSessionState.Unlock()
	}, nil
}

func activeIntegrationKindSession() *integrationKindSession {
	integrationKindSessionState.Lock()
	defer integrationKindSessionState.Unlock()
	return integrationKindSessionState.active
}

// acquireIntegrationCluster gives a namespaced chatbot scenario the shared
// da-platform (GH-2215). A running platform (a release run's, platform:up's, or
// one an earlier scenario in the aggregate session started) is reused without
// ownership; with none running, the scenario starts one it owns. The aggregate
// session adopts that ownership, so the platform lives until the session closes.
func acquireIntegrationCluster(root string) (kindrig.Cluster, error) {
	platform, err := kindrig.AcquirePlatform(kindrig.PlatformOptions{
		EvidenceDirectory: filepath.Join(root, "build", "kind-evidence",
			kindrig.PlatformClusterName+"-"+time.Now().UTC().Format("20060102T150405Z")),
	})
	if err != nil {
		return kindrig.Cluster{}, err
	}
	return platform.Detach(), nil
}

// releaseDirectScenarioCluster ends a scenario invoked outside the aggregate
// session: failure evidence first, then the scenario namespace, then the
// platform when this run created it. The namespace goes before the cluster so a
// platform someone else owns is left clean.
func releaseDirectScenarioCluster(
	cluster kindrig.Cluster,
	run kindrig.Runner,
	failed bool,
	evidence kindrig.FailureEvidence,
	cleanupNamespace *func() error,
) error {
	if failed {
		if err := evidence.Capture(run, cluster.Name); err != nil {
			fmt.Printf("kind: capture failure evidence for %s failed: %v\n", cluster.Name, err)
		}
	}
	err := (*cleanupNamespace)()
	*cleanupNamespace = func() error { return nil }
	cluster.Release(run)
	return err
}

// scenarioClusterGone reports whether a failed session scenario's cluster was
// deleted with the poisoned session, leaving no namespace to clean.
func scenarioClusterGone(name string) bool {
	return !kindrig.Exists(kindrig.CaptureRun, name)
}

func reusePreparedHostImage(
	run helmLLMCommandRunner,
	image string,
) (bool, error) {
	session := activeIntegrationKindSession()
	if session == nil {
		return false, nil
	}
	session.mu.Lock()
	expected := session.hostImages[image]
	session.mu.Unlock()
	if expected == "" {
		return false, nil
	}
	current, err := inspectHostImageID(run, image)
	if err != nil || current != expected {
		return false, nil
	}
	fmt.Printf("shared kind: reusing prepared host image %s digest=%s\n", image, current)
	return true, nil
}

func recordPreparedHostImage(
	run helmLLMCommandRunner,
	image string,
) error {
	session := activeIntegrationKindSession()
	if session == nil {
		return nil
	}
	imageID, err := inspectHostImageID(run, image)
	if err != nil {
		return err
	}
	session.mu.Lock()
	session.hostImages[image] = imageID
	session.mu.Unlock()
	return nil
}

func inspectHostImageID(run helmLLMCommandRunner, image string) (string, error) {
	output, err := run("docker", "image", "inspect", "--format={{.Id}}", image)
	if err != nil {
		return "", fmt.Errorf("inspect prepared host image %s: %w: %s",
			image, err, strings.TrimSpace(string(output)))
	}
	imageID := strings.TrimSpace(string(output))
	if !strings.HasPrefix(imageID, "sha256:") {
		return "", fmt.Errorf("prepared host image %s has unverified ID %q", image, imageID)
	}
	return imageID, nil
}

func registerAggregateFinalizer(name string, finalize func() error) bool {
	session := activeIntegrationKindSession()
	if session == nil {
		return false
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if _, exists := session.finalizers[name]; !exists {
		session.finalizers[name] = finalize
	}
	return true
}

func (session *integrationKindSession) takeFinalizers() []func() error {
	names := make([]string, 0, len(session.finalizers))
	for name := range session.finalizers {
		names = append(names, name)
	}
	sort.Strings(names)
	finalizers := make([]func() error, 0, len(names))
	for _, name := range names {
		finalizers = append(finalizers, session.finalizers[name])
	}
	session.finalizers = make(map[string]func() error)
	return finalizers
}

func runAggregateFinalizers(finalizers []func() error) error {
	var errs []error
	for _, finalize := range finalizers {
		if err := finalize(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// adoptAggregateKindCluster transfers an aggregate-created cluster to the
// active session. A direct integration target sees no session and retains its
// existing target-owned release behavior.
func adoptAggregateKindCluster(
	cluster kindrig.Cluster,
	run kindrig.Runner,
	evidence kindrig.FailureEvidence,
) bool {
	session := activeIntegrationKindSession()
	if session == nil {
		return false
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.cluster.Name == "" {
		session.cluster = cluster
		session.kindRun = run
		if evidence.Directory != "" {
			session.evidence = evidence
		}
		return true
	}
	if session.cluster.Name != cluster.Name {
		return false
	}
	session.kindRun = run
	if evidence.Directory != "" {
		session.evidence = evidence
	}
	return true
}

func retainAggregateKindCluster(
	cluster kindrig.Cluster,
	run kindrig.Runner,
	evidence kindrig.FailureEvidence,
) bool {
	return adoptAggregateKindCluster(cluster, run, evidence)
}

func releaseAggregateKindCluster(
	cluster kindrig.Cluster,
	run kindrig.Runner,
	evidence kindrig.FailureEvidence,
	cause error,
) bool {
	session := activeIntegrationKindSession()
	if session == nil {
		return false
	}
	if !adoptAggregateKindCluster(cluster, run, evidence) {
		return false
	}
	if cause != nil {
		if !session.deferBatchPoison() {
			session.poison(cause)
		}
	}
	return true
}

func (session *integrationKindSession) beginConcurrentBatch() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return errors.New("shared kind session is closed")
	}
	if session.poisoned != nil {
		return fmt.Errorf("shared kind session is poisoned: %w", session.poisoned)
	}
	if session.batching {
		return errors.New("shared kind session already has a concurrent batch")
	}
	session.batching = true
	return nil
}

func (session *integrationKindSession) deferBatchPoison() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.batching
}

func (session *integrationKindSession) endConcurrentBatch(cause error) {
	session.mu.Lock()
	session.batching = false
	session.mu.Unlock()
	if cause != nil {
		session.poison(cause)
	}
}

// prepareScenarioNamespace gives a chatbot scenario its own namespace on the
// shared platform through the kindrig scenario-namespace lifecycle.
func prepareScenarioNamespace(
	run kindrig.CommandRunner,
	scenario, release string,
) (string, func() error, error) {
	namespace, err := kindrig.PrepareScenarioNamespace(run, scenario, release)
	if err != nil {
		return "", nil, err
	}
	return namespace.Name, namespace.Release, nil
}

func (session *integrationKindSession) runTarget(name string, run func() error) error {
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return fmt.Errorf("%s: shared kind session is closed", name)
	}
	if session.poisoned != nil {
		err := session.poisoned
		session.mu.Unlock()
		return fmt.Errorf("%s: shared kind session is poisoned: %w", name, err)
	}
	session.mu.Unlock()

	started := time.Now()
	err := run()
	outcome := "passed"
	if err != nil {
		outcome = "failed"
		session.poison(err)
	}
	kindrig.LogPhase(kindrig.PlatformClusterName, "target", outcome, started, "scenario="+name)
	return err
}

func (session *integrationKindSession) poison(cause error) {
	session.mu.Lock()
	if session.poisoned == nil {
		session.poisoned = cause
	}
	cluster, run, evidence := session.cluster, session.kindRun, session.evidence
	finalizers := session.takeFinalizers()
	session.cluster = kindrig.Cluster{}
	session.mu.Unlock()

	if err := runAggregateFinalizers(finalizers); err != nil {
		fmt.Printf("shared kind: failure finalizer error: %v\n", err)
	}
	switch {
	case cluster.Name != "" && cluster.Created:
		cluster.ReleaseAfter(run, true, evidence)
	case cluster.Name != "":
		// A platform the session does not own stays up for its owner; keep the
		// evidence the deletion would otherwise have captured.
		if err := evidence.Capture(run, cluster.Name); err != nil {
			fmt.Printf("shared kind: capture failure evidence failed: %v\n", err)
		}
	}
}

func (session *integrationKindSession) close() {
	_ = session.closeWithError()
}

func (session *integrationKindSession) closeWithError() error {
	started := time.Now()
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return nil
	}
	session.closed = true
	cluster, run := session.cluster, session.kindRun
	finalizers := session.takeFinalizers()
	session.cluster = kindrig.Cluster{}
	session.mu.Unlock()

	finalizerErr := runAggregateFinalizers(finalizers)
	if finalizerErr != nil {
		fmt.Printf("shared kind: final teardown error: %v\n", finalizerErr)
	}
	if cluster.Name != "" {
		cluster.Release(run)
	}
	kindrig.LogPhase(kindrig.PlatformClusterName, "final-teardown", "complete", started, "")
	return finalizerErr
}

func integrationKindSessionRoot() string {
	root, err := os.Getwd()
	if err != nil {
		return "."
	}
	return root
}
