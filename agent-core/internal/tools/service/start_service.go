// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// validateStartService checks a start_service declaration at construction, so
// a malformed selector or bound fails the profile load rather than the first
// launch.
func validateStartService(name, init string, cfg ToolConfig) error {
	if cfg.Profile == "" {
		return fmt.Errorf("tool %q (%s) requires a profile", name, init)
	}
	pairs := []struct{ field, literal, from string }{
		{"service", cfg.Service, cfg.ServiceFrom},
		{"request", cfg.Request, cfg.RequestFrom},
		{"output", cfg.Output, cfg.OutputFrom},
	}
	for _, pair := range pairs {
		if pair.literal != "" && pair.from != "" {
			return fmt.Errorf("tool %q (%s) declares both %s and %s_from", name, init, pair.field, pair.field)
		}
		if pair.from == "" {
			continue
		}
		if _, _, ok := core.ParseFromSelector(pair.from); !ok {
			return fmt.Errorf("tool %q (%s) %s_from must be a $from(label).path selector", name, init, pair.field)
		}
	}
	if cfg.MaxRunning < 0 {
		return fmt.Errorf("tool %q (%s) max_running must not be negative", name, init)
	}
	return nil
}

// startService spawns one declared profile as a detached child and returns its
// handle without waiting for it to exit (srd040 R7.1). At a declared bound it
// starts nothing and reports the limit (R7.3).
func (c command) startService() core.Result {
	name, err := c.literalOrSelected(c.cfg.Service, c.cfg.ServiceFrom)
	if err != nil {
		return commandError(c.toolName, fmt.Errorf("%s: service_from: %w", c.toolName, err))
	}
	if name == "" {
		name = derivedServiceName(c.cfg.Profile, time.Now())
	}
	request, err := c.literalOrSelected(c.cfg.Request, c.cfg.RequestFrom)
	if err != nil {
		return commandError(c.toolName, fmt.Errorf("%s: request_from: %w", c.toolName, err))
	}
	output, err := c.literalOrSelected(c.cfg.Output, c.cfg.OutputFrom)
	if err != nil {
		return commandError(c.toolName, fmt.Errorf("%s: output_from: %w", c.toolName, err))
	}

	out, err := c.state.Start(StartSpec{
		Name: name, Binary: c.childBinary(), Profile: c.cfg.Profile, CoreRoot: c.coreRoot,
		Directory: c.cfg.Directory, Request: request, Output: output, Env: c.cfg.Env,
		MaxRunning: c.cfg.MaxRunning,
	})
	if errors.Is(err, ErrLimitReached) {
		return core.Result{
			Signal: SignalServiceLimitReached, CommandName: c.toolName,
			Output: jsonOutput(map[string]interface{}{
				"service": name, "running": c.state.RunningCount(), "max_running": c.cfg.MaxRunning,
			}),
		}
	}
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{
		Signal: SignalServiceStarted, CommandName: c.toolName,
		Output:  jsonOutput(out),
		Receipt: jsonOutput(map[string]interface{}{"service": name}),
	}
}

// listServices reports every tracked child with its outcome (srd040 R7.4).
func (c command) listServices() core.Result {
	return core.Result{
		Signal: SignalChildrenListed, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{"services": c.state.List()}),
	}
}

// literalOrSelected returns the literal, or the non-empty string a selector
// resolves to; both empty yields "".
func (c command) literalOrSelected(literal, selector string) (string, error) {
	if selector == "" {
		return literal, nil
	}
	value, err := core.ResolveFromSelector(c.commandState, selector)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("selector %q resolved to %T, want string", selector, value)
	}
	if text == "" {
		return "", fmt.Errorf("selector %q resolved to an empty string", selector)
	}
	return text, nil
}

// childBinary prefers the declaration, then --child-agent-binary, then the
// running executable, the order self_invoke uses.
func (c command) childBinary() string {
	if c.cfg.Binary != "" {
		return c.cfg.Binary
	}
	if c.childAgentBinary != "" {
		return c.childAgentBinary
	}
	return os.Args[0]
}

// derivedServiceName names an undeclared child after its profile and start
// time. A profile file named profile.yaml takes its directory's name, since
// agents/critic/profile.yaml is the critic, not "profile".
func derivedServiceName(profile string, at time.Time) string {
	base := strings.TrimSuffix(filepath.Base(profile), filepath.Ext(profile))
	if base == "profile" {
		base = filepath.Base(filepath.Dir(profile))
	}
	return fmt.Sprintf("%s-%d", base, at.UnixNano())
}
