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

const aggregateKindCluster = "da-chatbot-mesh-aggregate"
const aggregateNamespaceCleanupTimeout = "180s"

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
			Directory:  filepath.Join(root, "build", "kind-evidence", aggregateKindCluster),
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

func aggregateClusterName(standalone string) string {
	if activeIntegrationKindSession() != nil {
		return aggregateKindCluster
	}
	return standalone
}

func aggregateKindClusterOwned(name string) bool {
	session := activeIntegrationKindSession()
	if session == nil {
		return false
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.cluster.Name == name && session.cluster.Created
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

func prepareAggregateNamespace(
	run kindrig.CommandRunner,
	scenario, release string,
) (string, func() error, error) {
	if activeIntegrationKindSession() == nil {
		return "default", func() error { return nil }, nil
	}
	namespace := "da-" + scenario
	if output, err := run("kubectl", "create", "namespace", namespace); err != nil {
		return "", nil, fmt.Errorf("create aggregate namespace %s: %w: %s",
			namespace, err, output)
	}
	if output, err := run(
		"kubectl", "config", "set-context", "--current", "--namespace", namespace,
	); err != nil {
		_, _ = run("kubectl", "delete", "namespace", namespace,
			"--ignore-not-found=true", "--wait=true", "--timeout=60s")
		return "", nil, fmt.Errorf("select aggregate namespace %s: %w: %s",
			namespace, err, output)
	}
	cleanup := func() error {
		var cleanupErrors []error
		if output, err := run(
			"helm", "uninstall", release, "--namespace", namespace, "--ignore-not-found",
		); err != nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("uninstall %s/%s: %w: %s", namespace, release, err, output))
		}
		if output, err := run(
			"kubectl", "delete", "pod", "--all", "--namespace", namespace,
			"--ignore-not-found=true", "--wait=true", "--timeout=60s",
		); err != nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("drain aggregate namespace %s pods: %w: %s", namespace, err, output))
		}
		if output, err := run(
			"kubectl", "delete", "persistentvolumeclaim", "--all", "--namespace", namespace,
			"--ignore-not-found=true", "--wait=true", "--timeout=60s",
		); err != nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("delete aggregate namespace %s PVCs: %w: %s", namespace, err, output))
		}
		if output, err := run(
			"kubectl", "delete", "namespace", namespace,
			"--ignore-not-found=true", "--wait=false",
		); err != nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("delete aggregate namespace %s: %w: %s", namespace, err, output))
		}
		if output, err := run(
			"kubectl", "wait", "--for=delete", "namespace/"+namespace,
			"--timeout="+aggregateNamespaceCleanupTimeout,
		); err != nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("wait for aggregate namespace %s deletion: %w: %s",
					namespace, err, output))
		}
		if _, err := run("kubectl", "get", "namespace", namespace); err == nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("aggregate namespace %s remains after cleanup", namespace))
		}
		if err := verifyAggregateDataPlane(run); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
		return errors.Join(cleanupErrors...)
	}
	return namespace, cleanup, nil
}

func verifyAggregateDataPlane(run kindrig.CommandRunner) error {
	if activeIntegrationKindSession() == nil {
		return nil
	}
	checks := [][]string{
		{"kubectl", "-n", "kube-system", "wait", "--for=condition=Ready",
			"pod", "-l", "k8s-app=kube-proxy", "--timeout=120s"},
		{"kubectl", "-n", "kube-system", "rollout", "status",
			"deployment/coredns", "--timeout=120s"},
		{"kubectl", "get", "--raw=/readyz"},
	}
	for _, command := range checks {
		if output, err := run(command[0], command[1:]...); err != nil {
			return fmt.Errorf("shared kind data-plane readiness %s: %w: %s",
				strings.Join(command, " "), err, output)
		}
	}
	return nil
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
	kindrig.LogPhase(aggregateKindCluster, "target", outcome, started, "scenario="+name)
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
	if cluster.Name != "" && cluster.Created {
		cluster.ReleaseAfter(run, true, evidence)
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
	kindrig.LogPhase(aggregateKindCluster, "final-teardown", "complete", started, "")
	return finalizerErr
}

func integrationKindSessionRoot() string {
	root, err := os.Getwd()
	if err != nil {
		return "."
	}
	return root
}
