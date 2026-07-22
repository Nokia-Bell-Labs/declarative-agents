// Copyright (c) 2026 Nokia. All rights reserved.

// Package service provides the words a rig machine composes other machines
// with (srd040): background serve-mode child agents, concurrent validator
// runs, and scenario discovery. Every word is deterministic and calls no
// model.
package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"
)

const (
	defaultHealthTimeout  = 30 * time.Second
	defaultHealthInterval = 100 * time.Millisecond
	defaultStopGrace      = 3 * time.Second
	defaultRunTimeout     = 10 * time.Minute
)

// child is one running serve-mode agent process.
type child struct {
	name    string
	cmd     *exec.Cmd
	baseURL string
	done    chan struct{}
	once    sync.Once
}

// State holds the serve-mode children a rig started. Children are
// process-group managed so stopping one stops anything it spawned, and
// StopAll reaps the set when the parent shuts down (srd040 R1.4).
type State struct {
	mu       sync.Mutex
	children map[string]*child
}

// NewState returns an empty service state.
func NewState() *State {
	return &State{children: map[string]*child{}}
}

// StartSpec describes one serve-mode child.
type StartSpec struct {
	Name      string
	Binary    string
	Profile   string
	Directory string
	Request   string
	Address   string
	Env       []string
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
// its handle and base URL. It does not wait for the child to become healthy;
// AwaitHealthy does that.
func (s *State) Start(spec StartSpec) (map[string]interface{}, error) {
	if err := validateStartSpec(spec); err != nil {
		return nil, err
	}

	address, err := resolveAddress(spec.Address)
	if err != nil {
		return nil, fmt.Errorf("start_service %q: %w", spec.Name, err)
	}

	s.mu.Lock()
	if _, exists := s.children[spec.Name]; exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("start_service %q: a service with that name is already running", spec.Name)
	}
	s.mu.Unlock()

	cmd := childCommand(spec)
	if err := cmd.Start(); err != nil {
		// A spawn failure is a tool error, never a panic (srd040 R6.3).
		return nil, fmt.Errorf("start_service %q: %w", spec.Name, err)
	}

	entry := s.track(spec.Name, cmd, address)

	return map[string]interface{}{
		"service":  spec.Name,
		"pid":      cmd.Process.Pid,
		"address":  address,
		"base_url": entry.baseURL,
	}, nil
}

// AwaitHealthy polls url until it answers or the bounded timeout elapses. It
// never polls without a limit (srd040 R2.2), and reports healthy and timeout
// as distinct outcomes so a machine can route teardown on failure.
func (s *State) AwaitHealthy(url string, timeout, interval time.Duration) (map[string]interface{}, bool) {
	if timeout <= 0 {
		timeout = defaultHealthTimeout
	}
	if interval <= 0 {
		interval = defaultHealthInterval
	}
	client := &http.Client{
		Timeout:   interval * 4,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	defer client.CloseIdleConnections()

	deadline := time.Now().Add(timeout)
	attempts := 0
	for {
		attempts++
		resp, err := client.Get(url)
		if err == nil {
			status := resp.StatusCode
			_ = resp.Body.Close()
			if status < 400 {
				return map[string]interface{}{
					"url": url, "status": status, "attempts": attempts,
				}, true
			}
		}
		if time.Now().After(deadline) || time.Until(deadline) <= 0 {
			return map[string]interface{}{
				"url": url, "attempts": attempts, "timeout": timeout.String(),
			}, false
		}
		sleep := interval
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		time.Sleep(sleep)
	}
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

// Running reports the names of the services currently held.
func (s *State) Running() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.children))
	for name := range s.children {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *child) stop(grace time.Duration) map[string]interface{} {
	if grace <= 0 {
		grace = defaultStopGrace
	}
	out := map[string]interface{}{"service": c.name, "stopped": true}
	if c.cmd.Process == nil {
		return out
	}
	pid := c.cmd.Process.Pid

	// Signal the group, not just the leader, so a child's own children go too.
	c.once.Do(func() { _ = syscall.Kill(-pid, syscall.SIGTERM) })
	select {
	case <-c.done:
		out["signal"] = "SIGTERM"
	case <-time.After(grace):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-c.done
		out["signal"] = "SIGKILL"
		out["graceful"] = false
	}
	return out
}

func validateStartSpec(spec StartSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("start_service requires a service name")
	}
	if spec.Profile == "" {
		return fmt.Errorf("start_service %q requires a profile", spec.Name)
	}
	return nil
}

// track registers a started child and reaps it in the background, so Stop can
// wait on a closed channel rather than racing the process exit.
func (s *State) track(name string, cmd *exec.Cmd, address string) *child {
	entry := &child{
		name:    name,
		cmd:     cmd,
		baseURL: "http://" + address,
		done:    make(chan struct{}),
	}
	go func() {
		_ = cmd.Wait()
		close(entry.done)
	}()

	s.mu.Lock()
	s.children[name] = entry
	s.mu.Unlock()
	return entry
}

// resolveAddress returns the declared address, or a freshly reserved loopback
// port when none is declared or the port is left as 0.
func resolveAddress(declared string) (string, error) {
	if declared != "" && !hasZeroPort(declared) {
		return declared, nil
	}
	return FreeAddress()
}

// childCommand builds the serve-mode child process. The child gets its own
// process group, so stopping the service stops whatever it spawned rather
// than leaving a subtree behind.
func childCommand(spec StartSpec) *exec.Cmd {
	binary := spec.Binary
	if binary == "" {
		binary = "agent"
	}
	args := []string{"--profile", spec.Profile}
	if spec.Directory != "" {
		args = append(args, "--directory", spec.Directory)
	}
	if spec.Request != "" {
		args = append(args, "--request", spec.Request)
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
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
