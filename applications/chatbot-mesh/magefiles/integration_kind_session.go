// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const aggregateKindCluster = "da-chatbot-mesh-aggregate"

type integrationKindSession struct {
	mu       sync.Mutex
	root     string
	cluster  kindrig.Cluster
	kindRun  kindrig.Runner
	evidence kindrig.FailureEvidence
	poisoned error
	closed   bool
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
	return session.cluster.Name == cluster.Name
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
	session.cluster = kindrig.Cluster{}
	session.mu.Unlock()

	if cluster.Name != "" && cluster.Created {
		cluster.ReleaseAfter(run, true, evidence)
	}
}

func (session *integrationKindSession) close() {
	started := time.Now()
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return
	}
	session.closed = true
	cluster, run := session.cluster, session.kindRun
	session.cluster = kindrig.Cluster{}
	session.mu.Unlock()

	if cluster.Name != "" {
		cluster.Release(run)
	}
	kindrig.LogPhase(aggregateKindCluster, "final-teardown", "complete", started, "")
}

func integrationKindSessionRoot() string {
	root, err := os.Getwd()
	if err != nil {
		return "."
	}
	return root
}
