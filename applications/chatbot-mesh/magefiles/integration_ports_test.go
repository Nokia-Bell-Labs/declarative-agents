// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"net"
	"reflect"
	"strconv"
	"testing"
)

func TestSharedScenarioPortsAvoidLocalIntegrationOwnership(t *testing.T) {
	fixed := []int{11434, 18080, 18081, 18090, 18091, 18202}
	var listeners []net.Listener
	for _, port := range fixed {
		listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			listeners = append(listeners, listener)
		}
	}
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	ports, err := reserveLoopbackPorts(6)
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range ports {
		for _, owned := range fixed {
			if port == owned {
				t.Fatalf("dynamic shared port %d collided with local ownership", port)
			}
		}
	}
}

func TestPortForwardArgsMapDynamicLocalToDeclaredRemote(t *testing.T) {
	got := portForwardArgs("svc/example", []portForwardPair{
		{local: 29000, remote: 18080},
		{local: 29001, remote: 18081},
	})
	want := []string{
		"port-forward", "svc/example", "29000:18080", "29001:18081",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("port-forward args = %v, want %v", got, want)
	}
	if url := loopbackURL(29000, "/api/v1/chat"); url != "http://127.0.0.1:29000/api/v1/chat" {
		t.Fatalf("loopback URL = %q", url)
	}
}
