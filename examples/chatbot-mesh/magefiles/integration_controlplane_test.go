// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net"
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

func TestFakeDeploymentAPIBindFailureIsAnError(t *testing.T) {
	t.Parallel()
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	defer reservation.Close()
	address := reservation.Addr().String()

	_, err = startFakeDeploymentAPIOnAddr(&deploymentAPIRecorder{}, address)
	if err == nil || !strings.Contains(err.Error(), "bind fake deployment API") {
		t.Fatalf("startFakeDeploymentAPIOnAddr() error = %v, want bind failure", err)
	}
}

func TestFakeDeploymentAPIStopReleasesAddress(t *testing.T) {
	t.Parallel()
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
		t.Fatalf("start fake API: %v", err)
	}
	stop()

	rebound, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("fake API stop did not release %s: %v", address, err)
	}
	_ = rebound.Close()
}
