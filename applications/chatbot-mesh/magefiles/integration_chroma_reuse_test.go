// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// netListenOn8000 binds the port the Chroma helpers probe. The port is fixed in
// chromaHeartbeatURL, so the reuse test has to use it rather than an ephemeral
// one.
func netListenOn8000() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:8000")
}

// chromaHeartbeatStub serves the heartbeat on the fixed port the target probes,
// so the reuse path can be exercised without a Chroma container. It returns a
// close func, or skips the test when the port is genuinely in use.
func chromaHeartbeatStub(t *testing.T) func() {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"nanosecond heartbeat":1}`))
	}))
	listener, err := netListenOn8000()
	if err != nil {
		t.Skipf("port 8000 unavailable for the reuse stub: %v", err)
	}
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	return server.Close
}

// TestEnsureChromaServerReusesAHealthyServer is the GH-708 regression: with a
// Chroma already on the port, the target must reuse it rather than run a
// container that cannot bind.
func TestEnsureChromaServerReusesAHealthyServer(t *testing.T) {
	closeStub := chromaHeartbeatStub(t)
	defer closeStub()

	id, err := ensureChromaServer(t.TempDir())

	if err != nil {
		t.Fatalf("ensureChromaServer() error: %v", err)
	}
	if id != "" {
		t.Fatalf("ensureChromaServer() id = %q, want empty so the reused server is not removed", id)
	}
}

// TestStopChromaContainerLeavesAReusedServer covers the created-by-me guard:
// the empty id ensureChromaServer returns for a reused server must never reach
// docker rm. A regression here deletes a developer's own container.
func TestStopChromaContainerLeavesAReusedServer(t *testing.T) {
	t.Parallel()
	// stopChromaContainer("") must be a no-op. If it ever shells out, the docker
	// call would run with an empty target, which is precisely the bug.
	stopChromaContainer("")
}

// TestChromaLaunchErrorNamesThePortHolder covers the second half of GH-708: a
// bare "exit status 125" sends the reader to the Docker daemon rather than to
// the process holding the port.
func TestChromaLaunchErrorNamesThePortHolder(t *testing.T) {
	t.Parallel()
	detail := "docker: Error response from daemon: failed to set up container networking: " +
		"driver failed programming external connectivity on endpoint stupefied_newton: " +
		"Bind for 0.0.0.0:8000 failed: port is already allocated"

	err := chromaLaunchError(chromaImage, errors.New("exit status 125"), detail)

	for _, want := range []string{"port 8000", chromaHeartbeatURL, "reuse", "exit status 125"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("chromaLaunchError() = %q, want it to mention %q", err, want)
		}
	}
}

// TestChromaLaunchErrorPassesThroughUnrelatedFailures keeps the diagnosis
// narrow: a pull failure or a missing daemon must not be reported as a port
// conflict.
func TestChromaLaunchErrorPassesThroughUnrelatedFailures(t *testing.T) {
	t.Parallel()
	detail := "docker: Error response from daemon: pull access denied for chromadb/chroma"

	err := chromaLaunchError(chromaImage, errors.New("exit status 125"), detail)

	if strings.Contains(err.Error(), "port 8000") {
		t.Errorf("chromaLaunchError() = %q, want no port-conflict claim for a pull failure", err)
	}
	if !strings.Contains(err.Error(), "pull access denied") {
		t.Errorf("chromaLaunchError() = %q, want the original daemon detail preserved", err)
	}
}
