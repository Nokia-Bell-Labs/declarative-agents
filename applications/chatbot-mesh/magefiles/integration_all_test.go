// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestIntegrationLaneRosterSeparatesPolicyAndSharedTargets(t *testing.T) {
	targets := integrationTargets(Integration{})
	var localNames []string
	for _, target := range localIntegrationTargets(targets) {
		localNames = append(localNames, target.name)
		if target.sharedKind || target.name == "policyProof" {
			t.Fatalf("local lane includes isolated target %+v", target)
		}
	}
	for _, want := range []string{
		"chatbot", "chroma", "ragServer", "controlPlane",
		"embeddingExclusion", "observer", "rig", "applier",
	} {
		if !containsString(localNames, want) {
			t.Errorf("local lane missing %s: %v", want, localNames)
		}
	}
}

func TestSharedIntegrationBatchWaitsForEveryStartedFailure(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	started := make(chan string, 3)
	release := make(chan struct{})
	targetError := errors.New("controlled swap failure")
	targets := []integrationTarget{
		{name: "helmSmoke", sharedKind: true, fn: func() error { return nil }},
		{name: "helmSwap", sharedKind: true, fn: func() error {
			started <- "helmSwap"
			<-release
			return targetError
		}},
		{name: "helmLLMTier", sharedKind: true, fn: func() error {
			started <- "helmLLMTier"
			<-release
			return nil
		}},
		{name: "applierLive", sharedKind: true, fn: func() error {
			started <- "applierLive"
			<-release
			return nil
		}},
	}
	results := make(chan integrationResult, len(targets))
	done := make(chan struct{})
	go func() {
		runSharedIntegrationLane(session, targets, results)
		close(done)
	}()
	seen := map[string]bool{<-started: true, <-started: true, <-started: true}
	for _, name := range []string{"helmSwap", "helmLLMTier", "applierLive"} {
		if !seen[name] {
			t.Fatalf("concurrent batch did not start %s: %v", name, seen)
		}
	}
	close(release)
	<-done
	resultByName := make(map[string]error)
	for range targets {
		result := <-results
		resultByName[result.name] = result.err
	}
	if !errors.Is(resultByName["helmSwap"], targetError) {
		t.Fatalf("swap error = %v, want %v", resultByName["helmSwap"], targetError)
	}
	if resultByName["helmLLMTier"] != nil || resultByName["applierLive"] != nil {
		t.Fatalf("successful concurrent results hidden: %v", resultByName)
	}
	if session.poisoned == nil ||
		!strings.Contains(session.poisoned.Error(), "helmSwap") {
		t.Fatalf("batch failure did not poison session with attribution: %v", session.poisoned)
	}
}

func TestSharedIntegrationSmokeFailureBlocksTerminalBatch(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	smokeErr := errors.New("smoke failed")
	called := false
	targets := []integrationTarget{
		{name: "helmSmoke", sharedKind: true, fn: func() error { return smokeErr }},
		{name: "helmSwap", sharedKind: true, fn: func() error { called = true; return nil }},
		{name: "helmLLMTier", sharedKind: true, fn: func() error { called = true; return nil }},
		{name: "applierLive", sharedKind: true, fn: func() error { called = true; return nil }},
	}
	results := make(chan integrationResult, len(targets))
	runSharedIntegrationLane(session, targets, results)
	if called {
		t.Fatal("terminal shared target ran after smoke failure")
	}
	for range targets {
		result := <-results
		if result.name != "helmSmoke" &&
			(result.err == nil || !strings.Contains(result.err.Error(), "blocked by helmSmoke")) {
			t.Fatalf("blocked result = %+v", result)
		}
	}
}
