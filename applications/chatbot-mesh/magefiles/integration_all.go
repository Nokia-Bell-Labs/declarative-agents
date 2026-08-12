// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

type integrationTarget struct {
	name       string
	fn         func() error
	sharedKind bool
}

func integrationTargets(i Integration) []integrationTarget {
	return []integrationTarget{
		{name: "chatbot", fn: i.Chatbot},
		{name: "chroma", fn: i.Chroma},
		{name: "ragServer", fn: i.RagServer},
		{name: "controlPlane", fn: i.ControlPlane},
		{name: "embeddingExclusion", fn: i.EmbeddingExclusion},
		{name: "observer", fn: i.Observer},
		{name: "policyProof", fn: i.PolicyProof},
		{name: "rig", fn: i.Rig},
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "helmSwap", fn: i.HelmSwap, sharedKind: true},
		{name: "helmLLMTier", fn: i.HelmLLMTier, sharedKind: true},
		{name: "applier", fn: i.Applier},
		{name: "applierLive", fn: i.ApplierLive, sharedKind: true},
	}
}

// SharedSmokeSwap runs the two namespace-isolated scenarios used to measure
// and verify shared-session data-plane readiness without the unrelated local,
// policy, LLM-tier, and applier targets.
func (i Integration) SharedSmokeSwap() error {
	return runSharedKindTargets([]integrationTarget{
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "helmSwap", fn: i.HelmSwap, sharedKind: true},
	})
}

// SharedSmokeApplier prepares the default-CNI session through the smoke
// scenario, then measures applierLive on the same clean cluster. It is the
// focused repeatable gate for the warm applier target's release budget.
func (i Integration) SharedSmokeApplier() error {
	return runSharedKindTargets([]integrationTarget{
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "applierLive", fn: i.ApplierLive, sharedKind: true},
	})
}

// SharedApplierBenchmark prepares one smoke-proven session and runs the warm
// applier proof three times, matching the performance acceptance measurement
// without repeating unrelated application targets or cluster setup.
func (i Integration) SharedApplierBenchmark() error {
	return runSharedKindTargets([]integrationTarget{
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "applierLive-1", fn: i.ApplierLive, sharedKind: true},
		{name: "applierLive-2", fn: i.ApplierLive, sharedKind: true},
		{name: "applierLive-3", fn: i.ApplierLive, sharedKind: true},
	})
}

// SharedLLMBenchmark prepares one smoke-proven session, populates the
// identity-keyed model cache once, then records three warm LLM-tier runs.
func (i Integration) SharedLLMBenchmark() error {
	return runSharedKindTargets([]integrationTarget{
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "helmLLMTier-bootstrap", fn: i.HelmLLMTier, sharedKind: true},
		{name: "helmLLMTier-warm-1", fn: i.HelmLLMTier, sharedKind: true},
		{name: "helmLLMTier-warm-2", fn: i.HelmLLMTier, sharedKind: true},
		{name: "helmLLMTier-warm-3", fn: i.HelmLLMTier, sharedKind: true},
	})
}

func runSharedKindTargets(targets []integrationTarget) (resultErr error) {
	session := newIntegrationKindSession(integrationKindSessionRoot())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		return err
	}
	defer deactivate()
	defer func() {
		retainAggregateObservability()
		resultErr = errors.Join(resultErr, session.closeWithError())
	}()
	for _, target := range targets {
		if err := session.runTarget(target.name, target.fn); err != nil {
			return err
		}
	}
	return nil
}

// All runs every integration target this application owns and prints a
// pass/fail/skip summary, returning an error when any target fails. Each target
// self-skips (returns nil after printing SKIP) when an optional live
// prerequisite -- Docker, kind, Helm, or a local model server -- is missing, so
// the aggregate is portable to a machine without them while still exercising
// and gating every runnable target on capable hosts. This aggregate is what
// lets the released application participate in the repository release gate
// rather than being tagged without its own integration evidence (GH-1343).
func (i Integration) All() error {
	session := newIntegrationKindSession(integrationKindSessionRoot())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		return err
	}
	defer deactivate()
	targets := integrationTargets(i)
	results := make(chan integrationResult, len(targets))
	var lanes sync.WaitGroup
	lanes.Add(3)
	go func() {
		defer lanes.Done()
		runSerialIntegrationLane("local", localIntegrationTargets(targets), results)
	}()
	go func() {
		defer lanes.Done()
		runSerialIntegrationLane("policy", namedIntegrationTargets(targets, "policyProof"), results)
	}()
	go func() {
		defer lanes.Done()
		runSharedIntegrationLane(session, targets, results)
	}()
	lanes.Wait()
	retainAggregateObservability()
	cleanupErr := session.closeWithError()

	resultByName := make(map[string]integrationResult, len(targets))
	for range targets {
		result := <-results
		resultByName[result.name] = result
	}
	failed := 0
	fmt.Printf("\n%s\n", strings.Repeat("─", 40))
	for _, target := range targets {
		result := resultByName[target.name]
		if result.err != nil {
			failed++
			fmt.Printf("  FAIL  %s  %v\n", target.name, result.err)
			continue
		}
		fmt.Printf("  PASS  %s\n", target.name)
	}
	if cleanupErr != nil {
		fmt.Printf("  FAIL  final teardown  %v\n", cleanupErr)
	}
	fmt.Printf("%s\n", strings.Repeat("─", 40))
	var aggregateErrors []error
	if failed > 0 {
		aggregateErrors = append(aggregateErrors,
			fmt.Errorf("%d integration target(s) failed", failed))
	}
	if cleanupErr != nil {
		aggregateErrors = append(aggregateErrors,
			fmt.Errorf("aggregate final teardown failed: %w", cleanupErr))
	}
	return errors.Join(aggregateErrors...)
}

// retainAggregateObservability makes the completed aggregate session
// responsible for stopping its host collector. Registration happens only
// after all concurrent lanes finish, so failure cleanup cannot stop telemetry
// while another lane still needs it. The spool remains outside this lifecycle.
func retainAggregateObservability() {
	registerAggregateFinalizer("shared-observability", func() error {
		return (Observability{}).Down()
	})
}

type integrationResult struct {
	name string
	err  error
}

func localIntegrationTargets(targets []integrationTarget) []integrationTarget {
	var local []integrationTarget
	for _, target := range targets {
		if !target.sharedKind && target.name != "policyProof" {
			local = append(local, target)
		}
	}
	return local
}

func namedIntegrationTargets(
	targets []integrationTarget,
	names ...string,
) []integrationTarget {
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	var selected []integrationTarget
	for _, target := range targets {
		if wanted[target.name] {
			selected = append(selected, target)
		}
	}
	return selected
}

func runSerialIntegrationLane(
	name string,
	targets []integrationTarget,
	results chan<- integrationResult,
) {
	started := time.Now()
	outcome := "passed"
	for _, target := range targets {
		result := runIntegrationTarget(target)
		results <- result
		if result.err != nil {
			outcome = "failed"
		}
	}
	kindrig.LogPhase("chatbot-integration", "lane", outcome, started, "lane="+name)
}

func runSharedIntegrationLane(
	session *integrationKindSession,
	targets []integrationTarget,
	results chan<- integrationResult,
) {
	started := time.Now()
	outcome := "passed"
	smoke := namedIntegrationTargets(targets, "helmSmoke")[0]
	fmt.Printf("\n=== %s ===\n", smoke.name)
	smokeErr := session.runTarget(smoke.name, smoke.fn)
	results <- integrationResult{name: smoke.name, err: smokeErr}
	terminal := namedIntegrationTargets(
		targets, "helmSwap", "helmLLMTier", "applierLive")
	if smokeErr != nil {
		outcome = "failed"
		for _, target := range terminal {
			results <- integrationResult{
				name: target.name,
				err:  fmt.Errorf("blocked by helmSmoke: %w", smokeErr),
			}
		}
		kindrig.LogPhase(
			"chatbot-integration", "lane", outcome, started, "lane=shared-kind")
		return
	}
	if err := session.beginConcurrentBatch(); err != nil {
		outcome = "failed"
		for _, target := range terminal {
			results <- integrationResult{name: target.name, err: err}
		}
		kindrig.LogPhase(
			"chatbot-integration", "lane", outcome, started, "lane=shared-kind")
		return
	}
	batchResults := make(chan integrationResult, len(terminal))
	var batch sync.WaitGroup
	for _, target := range terminal {
		target := target
		batch.Add(1)
		go func() {
			defer batch.Done()
			batchResults <- runIntegrationTarget(target)
		}()
	}
	batch.Wait()
	close(batchResults)
	var batchErrors []error
	for result := range batchResults {
		results <- result
		if result.err != nil {
			outcome = "failed"
			batchErrors = append(batchErrors,
				fmt.Errorf("%s: %w", result.name, result.err))
		}
	}
	session.endConcurrentBatch(errors.Join(batchErrors...))
	kindrig.LogPhase(
		"chatbot-integration", "lane", outcome, started, "lane=shared-kind")
}

func runIntegrationTarget(target integrationTarget) integrationResult {
	fmt.Printf("\n=== %s ===\n", target.name)
	started := time.Now()
	err := target.fn()
	outcome := "passed"
	if err != nil {
		outcome = "failed"
	}
	kindrig.LogPhase(
		"chatbot-integration", "target", outcome, started, "scenario="+target.name)
	return integrationResult{name: target.name, err: err}
}
