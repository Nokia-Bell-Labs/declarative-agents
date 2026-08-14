// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
)

const serverOwnershipBytes = 32

type serverLaunchIdentity struct {
	address   string
	ownership string
}

// Launch starts a configured REST server without waiting for requests.
func (s *ServerState) Launch(def ServerDefinition) (map[string]interface{}, error) {
	output, _, err := s.launchOwned(def)
	return output, err
}

func (s *ServerState) launchOwned(
	def ServerDefinition,
) (map[string]interface{}, serverLaunchIdentity, error) {
	runtime, err := newServerRuntime(def)
	if err != nil {
		return nil, serverLaunchIdentity{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.servers[def.Name]; exists {
		_ = runtime.listener.Close()
		return nil, serverLaunchIdentity{}, fmt.Errorf("REST server %q is already launched", def.Name)
	}
	s.servers[def.Name] = runtime
	go serveRuntime(runtime)
	return runtime.launchOutput(), serverLaunchIdentity{
		address: runtime.listener.Addr().String(), ownership: runtime.ownership,
	}, nil
}

// UndoLaunch stops only the live process-local listener identified by a
// validated launch receipt. A reconstructed process has no such listener, so
// absence is an already-compensated success.
func (s *ServerState) UndoLaunch(
	name, address, ownership string,
) (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime, ok := s.servers[name]
	if !ok {
		return map[string]interface{}{
			"server": name, "address": address, "status": "already_compensated",
		}, nil
	}
	if err := validateOwnedServerRuntime(runtime, name, address, ownership); err != nil {
		return nil, err
	}
	output, err := stopOwnedServerRuntime(runtime)
	delete(s.servers, name)
	return output, err
}

func validateOwnedServerRuntime(
	runtime *serverRuntime,
	name, address, ownership string,
) error {
	if !runtime.owned {
		return fmt.Errorf("REST server %q listener is not process-owned", name)
	}
	if runtime.name != name {
		return fmt.Errorf("REST server %q listener identity is %q", name, runtime.name)
	}
	actualAddress := runtime.listener.Addr().String()
	if actualAddress != address {
		return fmt.Errorf(
			"REST server %q listener address %q does not match receipt address %q",
			name, actualAddress, address,
		)
	}
	if runtime.ownership != ownership {
		return fmt.Errorf("REST server %q listener is not owned by the launch receipt", name)
	}
	return nil
}

func stopOwnedServerRuntime(runtime *serverRuntime) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runtime.stopTimeout())
	defer cancel()
	runtime.closeStopped()
	shutdownErr := runtime.httpServer.Shutdown(ctx)
	output := runtime.stopOutput()
	if shutdownErr != nil {
		return output, fmt.Errorf("shutdown REST server %q: %w", runtime.name, shutdownErr)
	}
	return output, nil
}

func newServerRuntime(def ServerDefinition) (*serverRuntime, error) {
	def.Server.Endpoints = injectLifecycleExit(def.Server)
	if err := validateRouteConflicts(def.Server.Endpoints); err != nil {
		return nil, err
	}
	mock, err := newMockState(def.Server.Endpoints)
	if err != nil {
		return nil, err
	}
	ownership, err := newServerOwnership(def.Name)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", def.Server.Address)
	if err != nil {
		return nil, fmt.Errorf("bind REST server %q: %w", def.Name, err)
	}
	runtime := &serverRuntime{
		name: def.Name, def: def, mock: mock, listener: listener, stopped: make(chan struct{}),
		runner: machineRequestRunner(def.MachineRequestRunner), requestMonitor: serverRequestMonitor(def),
		queue: make(chan InboundEvent, queueCapacity(def.Server.Queue)),
		owned: true, ownership: ownership,
	}
	runtime.httpServer = &http.Server{
		Handler: runtime, ReadTimeout: parseDuration(def.Limits.ReadTimeout, 0),
		ReadHeaderTimeout: parseDuration(def.Limits.ConnectTimeout, 0),
		MaxHeaderBytes:    def.Limits.MaxHeaderBytes,
	}
	return runtime, nil
}

func newServerOwnership(name string) (string, error) {
	value := make([]byte, serverOwnershipBytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create REST server %q ownership: %w", name, err)
	}
	return hex.EncodeToString(value), nil
}

func serverRequestMonitor(def ServerDefinition) monitor.RuntimeRecorder {
	if def.Monitor.Recorder != nil {
		return def.Monitor.Recorder
	}
	if def.Monitor.Store != nil {
		return monitor.NewRecorder(def.Monitor.Store, nil)
	}
	return nil
}
