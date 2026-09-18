// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package service provides the child-process words a machine composes other
// machines with (srd040): generic detached children under a declared bound,
// background serve-mode scenario children, one-validator child execution, and
// scenario discovery. Every word is deterministic and calls no model.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
)

const (
	defaultStopGrace  = 3 * time.Second
	defaultRunTimeout = 10 * time.Minute
)

// child is one tracked agent process. The reaper goroutine records its exit
// under State.mu, and an exited child stays tracked until it is stopped or
// reaped so list_services can report its outcome (srd040 R7.4).
type child struct {
	name      string
	process   *subprocess.Handle
	pid       int
	baseURL   string
	startedAt time.Time
	done      chan struct{}
	once      sync.Once

	exited     bool
	exitCode   int
	finishedAt time.Time
}

// ErrLimitReached reports that a bounded start found the declared number of
// live children already running (srd040 R7.3).
var ErrLimitReached = errors.New("running child limit reached")

// State holds the serve-mode children a rig started. Children are
// process-group managed so stopping one stops anything it spawned, and
// StopAll reaps the set when the parent shuts down (srd040 R1.4).
type State struct {
	mu       sync.Mutex
	children map[string]*child
	pending  map[string]struct{}
	ctx      context.Context
}

// NewState returns an empty service state.
func NewState() *State {
	return NewStateWithContext(context.Background())
}

// NewStateWithContext binds every child to the owning agent lifecycle.
func NewStateWithContext(ctx context.Context) *State {
	if ctx == nil {
		ctx = context.Background()
	}
	return &State{children: map[string]*child{}, pending: map[string]struct{}{}, ctx: ctx}
}

// StartSpec describes one serve-mode child.
type StartSpec struct {
	Name      string
	Binary    string
	Profile   string
	CoreRoot  string
	Directory string
	Request   string
	Output    string
	Address   string
	Env       []string

	// MaxRunning, when positive, bounds the live children the state holds;
	// Start returns ErrLimitReached at the bound without spawning.
	MaxRunning int
}

// FreeAddress reserves a loopback port and releases it, so a child can bind
// it. The gap between reserve and bind is the same one every port-picking
// harness carries.
func FreeAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("reserve port: %w", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", fmt.Errorf("release reserved port: %w", err)
	}
	return addr, nil
}

// Start launches one serve-mode child in its own process group and returns
// its handle and base URL. Readiness is probed by an ordinary REST word in the
// composing machine, not by this process-lifecycle boundary.
func (s *State) Start(spec StartSpec) (map[string]interface{}, error) {
	if err := validateStartSpec(spec); err != nil {
		return nil, err
	}

	address, err := resolveAddress(spec.Address)
	if err != nil {
		return nil, fmt.Errorf("start child %q: %w", spec.Name, err)
	}

	if err := s.reserve(spec); err != nil {
		return nil, err
	}
	process, err := subprocess.Start(s.ctx, childProcessSpec(spec))
	if err != nil {
		s.release(spec.Name)
		// A spawn failure is a tool error, never a panic (srd040 R6.3).
		return nil, fmt.Errorf("start child %q: %w", spec.Name, err)
	}

	entry := s.track(spec.Name, process, address)

	return map[string]interface{}{
		"service":    spec.Name,
		"pid":        entry.pid,
		"address":    address,
		"base_url":   entry.baseURL,
		"started_at": entry.startedAt.Format(time.RFC3339Nano),
	}, nil
}

// reserve claims the name and, under a declared bound, a live-child slot in
// one critical section, so concurrent request runs cannot both pass the bound
// check before either spawns.
func (s *State) reserve(spec StartSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.children[spec.Name]
	if _, starting := s.pending[spec.Name]; exists || starting {
		return fmt.Errorf("start child %q: a service with that name is already running", spec.Name)
	}
	if spec.MaxRunning > 0 && s.liveLocked()+len(s.pending) >= spec.MaxRunning {
		return fmt.Errorf("start child %q: %w (%d)", spec.Name, ErrLimitReached, spec.MaxRunning)
	}
	s.pending[spec.Name] = struct{}{}
	return nil
}

func (s *State) release(name string) {
	s.mu.Lock()
	delete(s.pending, name)
	s.mu.Unlock()
}

// Stop ends one service: a graceful signal to the process group, a bounded
// wait, then a kill (srd040 R3.1). Stopping an unknown or already-stopped
// service succeeds, so teardown paths are idempotent (R3.2).
func (s *State) Stop(name string, grace time.Duration) map[string]interface{} {
	s.mu.Lock()
	entry, ok := s.children[name]
	delete(s.children, name)
	s.mu.Unlock()
	if !ok {
		return map[string]interface{}{"service": name, "stopped": false, "reason": "not running"}
	}
	return entry.stop(grace)
}

// StopAll stops every running service, so a rig's children are reaped rather
// than orphaned when it shuts down (srd040 R1.4).
func (s *State) StopAll(grace time.Duration) []map[string]interface{} {
	s.mu.Lock()
	names := make([]string, 0, len(s.children))
	for name := range s.children {
		names = append(names, name)
	}
	s.mu.Unlock()
	sort.Strings(names)

	out := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		out = append(out, s.Stop(name, grace))
	}
	return out
}

// Reap stops every child with the package's declared graceful-stop bound. It is
// the process-shutdown safety net and the implementation of stop_all_services.
func (s *State) Reap() []map[string]interface{} {
	return s.StopAll(defaultStopGrace)
}

// Running reports the names of the tracked children that have not exited.
func (s *State) Running() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.children))
	for name, entry := range s.children {
		if !entry.exited {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// RunningCount reports how many tracked children have not exited.
func (s *State) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.liveLocked()
}

func (s *State) liveLocked() int {
	live := 0
	for _, entry := range s.children {
		if !entry.exited {
			live++
		}
	}
	return live
}

// List reports every tracked child, running or exited, sorted by name
// (srd040 R7.4).
func (s *State) List() []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.children))
	for name := range s.children {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		entry := s.children[name]
		item := map[string]interface{}{
			"service":    name,
			"pid":        entry.pid,
			"base_url":   entry.baseURL,
			"started_at": entry.startedAt.Format(time.RFC3339Nano),
			"status":     "running",
		}
		if entry.exited {
			item["status"] = "exited"
			item["exit_code"] = entry.exitCode
			item["finished_at"] = entry.finishedAt.Format(time.RFC3339Nano)
		}
		out = append(out, item)
	}
	return out
}

func (c *child) stop(grace time.Duration) map[string]interface{} {
	if grace <= 0 {
		grace = defaultStopGrace
	}
	out := map[string]interface{}{"service": c.name, "stopped": true}
	if c.process == nil {
		return out
	}
	// An exited child has nothing left to signal, and its process group id
	// may already name another process.
	select {
	case <-c.done:
		out["exited"] = true
		return out
	default:
	}

	// Signal the group, not just the leader, so a child's own children go too.
	c.once.Do(func() { _ = c.process.SignalGroup(syscall.SIGTERM) })
	select {
	case <-c.done:
		out["signal"] = "SIGTERM"
	case <-time.After(grace):
		_ = c.process.SignalGroup(syscall.SIGKILL)
		<-c.done
		out["signal"] = "SIGKILL"
		out["graceful"] = false
	}
	return out
}

func validateStartSpec(spec StartSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("start child requires a service name")
	}
	if spec.Profile == "" {
		return fmt.Errorf("start child %q requires a profile", spec.Name)
	}
	return nil
}

// track registers a started child and reaps it in the background, so Stop can
// wait on a closed channel rather than racing the process exit.
func (s *State) track(name string, process *subprocess.Handle, address string) *child {
	entry := &child{
		name:      name,
		process:   process,
		pid:       process.PID(),
		baseURL:   "http://" + address,
		startedAt: time.Now().UTC(),
		done:      make(chan struct{}),
	}
	go func() {
		code := exitCode(process.Wait())
		s.mu.Lock()
		entry.exited = true
		entry.exitCode = code
		entry.finishedAt = time.Now().UTC()
		s.mu.Unlock()
		close(entry.done)
	}()

	s.mu.Lock()
	delete(s.pending, name)
	s.children[name] = entry
	s.mu.Unlock()
	return entry
}

// exitCode reads a reaped process's exit status; -1 means it ended without
// one, such as by a signal or a wait failure.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// resolveAddress returns the declared address, or a freshly reserved loopback
// port when none is declared or the port is left as 0.
func resolveAddress(declared string) (string, error) {
	if declared != "" && !hasZeroPort(declared) {
		return declared, nil
	}
	return FreeAddress()
}

// childProcessSpec uses the canonical agent argument builder while leaving
// process-group and cancellation setup to the shared subprocess transport.
func childProcessSpec(spec StartSpec) subprocess.StartSpec {
	binary := spec.Binary
	if binary == "" {
		binary = "agent"
	}
	cfg := execute.Config{
		Binary: binary, Profile: spec.Profile, CoreRoot: spec.CoreRoot,
		Directory: spec.Directory, Request: spec.Request, Output: spec.Output, Env: spec.Env,
	}
	return subprocess.StartSpec{Binary: binary, Args: cfg.BuildArgs(), Env: cfg.Env}
}

func hasZeroPort(address string) bool {
	_, port, err := net.SplitHostPort(address)
	return err == nil && port == "0"
}

func jsonOutput(payload interface{}) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("{\"error\":%q}", err.Error())
	}
	return string(data)
}
