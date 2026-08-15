// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Monitor proves embedded monitor service wiring through cmd/agent.
func (Integration) Monitor() error {
	beginUC("monitor")
	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("monitor: resolve module root: %w", err)
	}
	cmd := exec.Command(
		"go", "test", "./cmd/agent", "./internal/tools/rest",
		"-run", "TestMonitorREST_FactoryUsesLiveMonitorState",
		"-count=1",
	)
	cmd.Dir = rootDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("monitor: embedded service proof failed: %w", err)
	}
	binary, err := buildFreshAgentFor("monitor")
	if err != nil {
		return err
	}
	addr, err := freeLoopbackAddress()
	if err != nil {
		return fmt.Errorf("monitor: reserve loopback address: %w", err)
	}
	profilePath, cleanup, err := prepareMonitorIntegrationRun(rootDir, addr)
	if err != nil {
		return fmt.Errorf("monitor: prepare profile: %w", err)
	}
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent := exec.CommandContext(ctx, binary, "--profile", profilePath)
	agent.Dir = rootDir
	var output bytes.Buffer
	agent.Stdout = &output
	agent.Stderr = &output
	if err := agent.Start(); err != nil {
		return fmt.Errorf("monitor: start agent: %w", err)
	}
	resultCh := make(chan error, 1)
	go func() { resultCh <- agent.Wait() }()
	defer func() {
		cancel()
		if agent.Process != nil {
			_ = agent.Process.Kill()
		}
	}()

	baseURL := "http://" + addr
	if err := waitMonitorHTTP(baseURL + "/monitor/state"); err != nil {
		return fmt.Errorf("monitor: service did not become ready: %w\n%s", err, output.String())
	}
	if err := postJSONStatus(baseURL+"/monitor/control/exit", `{"reason":"monitor integration complete"}`, http.StatusAccepted); err != nil {
		return fmt.Errorf("monitor: request exit: %w", err)
	}
	select {
	case err := <-resultCh:
		if err != nil {
			return fmt.Errorf("monitor: agent failed: %w\n%s", err, output.String())
		}
	case <-time.After(10 * time.Second):
		return fmt.Errorf("monitor: agent did not exit after control request\n%s", output.String())
	}
	fmt.Println("monitor: PASS - embedded service records and serves live state")
	return nil
}

func prepareMonitorIntegrationRun(rootDir, addr string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "monitor-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	restData, err := os.ReadFile(coreIntegrationProfilePath(rootDir, "monitor/rest.yaml"))
	if err != nil {
		cleanup()
		return "", nil, err
	}
	rest := strings.Replace(string(restData), "address: 127.0.0.1:0", "address: "+addr, 1)
	if rest == string(restData) {
		cleanup()
		return "", nil, fmt.Errorf("monitor fixture has no ephemeral address placeholder")
	}
	restPath := filepath.Join(tmpDir, "rest.yaml")
	if err := os.WriteFile(restPath, []byte(rest), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	profile := fmt.Sprintf(`name: monitor
machine: %s
tools:
  - %s
tool_declarations:
  - %s
rest_definitions:
  - %s
`, coreIntegrationProfilePath(rootDir, "monitor/machine.yaml"),
		coreIntegrationProfilePath(rootDir, "monitor/tools.yaml"),
		coreIntegrationProfilePath(rootDir, "monitor/declarations.yaml"),
		restPath)
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	return profilePath, cleanup, nil
}
