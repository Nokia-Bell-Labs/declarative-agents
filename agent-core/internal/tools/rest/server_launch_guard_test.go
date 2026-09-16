// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// The name guard on ServerState.Launch: a duplicate name is refused before
// anything binds, and a concurrent pair of launches registers exactly one.

package rest

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRESTServer_DuplicateLaunchNeverBindsItsAddress holds the reservation open
// across the duplicate launch, which is only possible because the name guard
// runs before the bind: a launch that bound first would report "address
// already in use" and never reach the guard. Holding it is also what makes the
// test deterministic — the earlier version closed the reservation to leave the
// port free and lost the race to whatever took it next (GH-2029).
func TestRESTServer_DuplicateLaunchNeverBindsItsAddress(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: binds real loopback listeners")
	}
	t.Parallel()
	state := NewServerState()
	_, err := state.Launch(ServerDefinition{Name: "duplicate", Server: monitorServer("duplicate")})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = state.Stop("duplicate") })

	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = reservation.Close() }()
	duplicate := monitorServer("duplicate")
	duplicate.Address = reservation.Addr().String()

	_, err = state.Launch(ServerDefinition{Name: "duplicate", Server: duplicate})

	require.ErrorContains(t, err, `REST server "duplicate" is already launched`,
		"a bind error here would mean the launch tried to take the reserved port")
	_ = reservation.Close()
	rebound, err := net.Listen("tcp", duplicate.Address)
	require.NoError(t, err, "duplicate launch left a listener at %s", duplicate.Address)
	require.NoError(t, rebound.Close())
}

// TestRESTServer_ConcurrentLaunchesRegisterOne covers the window the first
// check cannot close: the bind sits between it and the registration, so two
// launches of one name can both pass the first check. Exactly one wins, and
// the losers report the name guard rather than replacing the winner.
func TestRESTServer_ConcurrentLaunchesRegisterOne(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: binds real loopback listeners")
	}
	t.Parallel()
	state := NewServerState()
	const launches = 8
	start := make(chan struct{})
	errs := make(chan error, launches)

	for range launches {
		go func() {
			<-start
			_, err := state.Launch(ServerDefinition{Name: "raced", Server: monitorServer("raced")})
			errs <- err
		}()
	}
	close(start)

	succeeded := 0
	for range launches {
		err := <-errs
		if err == nil {
			succeeded++
			continue
		}
		require.ErrorContains(t, err, `REST server "raced" is already launched`)
	}
	require.Equal(t, 1, succeeded, "exactly one launch registers the name")
	_, err := state.Stop("raced")
	require.NoError(t, err)
	require.False(t, state.launched("raced"))
}
