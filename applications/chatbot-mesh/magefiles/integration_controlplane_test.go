// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// TestControlPlaneBodyIsClean pins the authority-boundary check: a deployment-API
// request body carrying the decided operation (operation/role/collection/patch) is
// clean, while any transport-authority field (a host, URL, method, or credential)
// makes it dirty. This is the check the control-plane tracer applies to every call
// the creator sends, enforcing that no endpoint crosses the boundary (srd005 R5.3).
func TestControlPlaneBodyIsClean(t *testing.T) {
	clean := []map[string]interface{}{
		{"operation": "ingest", "role": "corpus-ingest", "collection": "corpus2"},
		{"operation": "restart", "role": "chatbot", "patch": map[string]interface{}{"add_rag": "rag2"}},
		{},
	}
	for i, body := range clean {
		if !controlPlaneBodyIsClean(body) {
			t.Errorf("clean[%d] %v should be clean", i, body)
		}
	}
	dirty := []map[string]interface{}{
		{"operation": "ingest", "url": "http://rag0:18085"},
		{"operation": "restart", "host": "chatbot"},
		{"operation": "restart", "method": "POST"},
		{"operation": "ingest", "token": "secret"},
		{"operation": "ingest", "credential": "x"},
		{"base_url": "http://x"},
	}
	for i, body := range dirty {
		if controlPlaneBodyIsClean(body) {
			t.Errorf("dirty[%d] %v should carry a transport-authority field", i, body)
		}
	}
}

func TestApplicationRequestMachinesClassifyTerminalStatus(t *testing.T) {
	for _, test := range []struct {
		agent     string
		machine   string
		succeeded []string
		failed    []string
	}{
		{
			agent:     "provisioning-workflow-orchestrator",
			machine:   "request-machine.yaml",
			succeeded: []string{"Reconfigured"},
			failed:    []string{"Rejected", "Failed"},
		},
		{
			agent:     "creator",
			machine:   "request-machine.yaml",
			succeeded: []string{"HealthReported", "StateReported", "Ingested"},
			failed:    []string{"IngestRejected", "IngestFailed", "Failed"},
		},
		{
			agent:     "chatbot",
			machine:   "request-machine.yaml",
			succeeded: []string{"LLMResponded"},
			failed:    []string{"Failed"},
		},
		{
			agent:     "rag-server",
			machine:   "request-machine.yaml",
			succeeded: []string{"QueryResponded"},
			failed:    []string{"QueryRejected", "Failed"},
		},
		{
			agent:     "provisioning-workflow-orchestrator",
			machine:   "rollout-machine.yaml",
			succeeded: []string{"StatusRead"},
			failed:    []string{"Failed"},
		},
		{
			agent:     "provisioning-workflow-orchestrator",
			machine:   "state-machine.yaml",
			succeeded: []string{"StateRead"},
			failed:    []string{"Failed"},
		},
		{
			agent:     "applier",
			machine:   "state-machine.yaml",
			succeeded: []string{"Read"},
			failed:    []string{"Unavailable"},
		},
	} {
		t.Run(test.agent+"/"+test.machine, func(t *testing.T) {
			var machine struct {
				States []struct {
					Name      string `yaml:"name"`
					RunStatus string `yaml:"run_status"`
				} `yaml:"states"`
			}
			readIntakeYAML(t, filepath.Join(agentDir(t, test.agent), test.machine), &machine)
			statuses := map[string]string{}
			for _, state := range machine.States {
				statuses[state.Name] = state.RunStatus
			}
			for _, state := range test.succeeded {
				if statuses[state] != "succeeded" {
					t.Errorf("%s run_status = %q, want succeeded", state, statuses[state])
				}
			}
			for _, state := range test.failed {
				if statuses[state] != "failed" {
					t.Errorf("%s run_status = %q, want failed", state, statuses[state])
				}
			}
		})
	}
}

func TestDynamicFakeDeploymentAPIStartsWhileDefaultPortIsOccupied(t *testing.T) {
	reservation, err := net.Listen("tcp", cpDeploymentAPIAddr)
	if err == nil {
		defer func() { _ = reservation.Close() }()
	} else {
		// An ambient listener reproduces the release failure just as well as the
		// reservation this test normally owns.
		t.Logf("default port already occupied: %v", err)
	}
	stop, apiURL, err := startFakeDeploymentAPI(&deploymentAPIRecorder{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if strings.HasSuffix(apiURL, ":18090") {
		t.Fatalf("dynamic fake API reused the production default: %s", apiURL)
	}
	resp, err := http.Get(apiURL + "/provisioning/api/state")
	if err != nil {
		t.Fatalf("dynamic fake API is unreachable: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("dynamic fake API status = %d, want 200", resp.StatusCode)
	}
}

func TestFakeDeploymentAPIBindFailureIsAnError(t *testing.T) {
	t.Parallel()
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	defer func() { _ = reservation.Close() }()
	address := reservation.Addr().String()

	_, err = startFakeDeploymentAPIOnAddr(&deploymentAPIRecorder{}, address)
	if err == nil || !strings.Contains(err.Error(), "bind fake deployment API") {
		t.Fatalf("startFakeDeploymentAPIOnAddr() error = %v, want bind failure", err)
	}
}

// fakeAPIReleasesItsAddress runs one reserve, start, stop, rebind cycle and
// reports whether the rebind succeeded, distinguishing a lost port from a
// genuine failure to release.
//
// The address comes from the shared ephemeral range and is unowned twice in the
// cycle: between releasing the reservation and the API's bind, and between
// stop() and the rebind. Anything else on the machine may take it in either
// window, which made this test fail about half the time under a full parallel
// run with "bind: address already in use" -- a result that says nothing about
// whether stop() releases. Losing the port is retried; a real failure is not.
func fakeAPIReleasesItsAddress(t *testing.T) (ok bool) {
	t.Helper()
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := reservation.Addr().String()
	if err := reservation.Close(); err != nil {
		t.Fatalf("release reservation: %v", err)
	}
	stop, err := startFakeDeploymentAPIOnAddr(&deploymentAPIRecorder{}, address)
	if err != nil {
		return false // the port was taken before the API could bind it
	}
	stop()

	rebound, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	_ = rebound.Close()
	return true
}

func TestFakeDeploymentAPIStopReleasesAddress(t *testing.T) {
	t.Parallel()
	for attempt := 0; attempt < 10; attempt++ {
		if fakeAPIReleasesItsAddress(t) {
			return
		}
	}
	t.Fatal("fake API stop never released its address in 10 attempts; " +
		"a persistent failure here means stop() leaks the listener rather than losing a race")
}
