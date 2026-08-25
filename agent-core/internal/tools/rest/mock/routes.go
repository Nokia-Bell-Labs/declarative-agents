// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package mock

import (
	"fmt"
	"os"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/envexpand"
	"gopkg.in/yaml.v3"
)

const bindingMock = "mock"

// LoadRoutes reads and validates one endpoint's routes from its fixture file
// and inline config.
func LoadRoutes(name string, cfg *Config) ([]Route, error) {
	if cfg == nil {
		return nil, fmt.Errorf("endpoint %q binding %s requires mock config with fixtures or routes", name, bindingMock)
	}
	routes := append([]Route{}, cfg.Routes...)
	if cfg.Fixtures != "" {
		fileRoutes, err := loadFixture(cfg.Fixtures)
		if err != nil {
			return nil, fmt.Errorf("endpoint %q mock fixture: %w", name, err)
		}
		routes = append(routes, fileRoutes...)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("endpoint %q mock config declares no routes", name)
	}
	if err := validateRoutes(name, routes); err != nil {
		return nil, err
	}
	return routes, nil
}

func loadFixture(path string) ([]Route, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var fixture Fixture
	if err := yaml.Unmarshal(envexpand.Expand(data), &fixture); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return fixture.Routes, nil
}

func validateRoutes(name string, routes []Route) error {
	seen := map[string]bool{}
	for i, route := range routes {
		if err := validateRoute(name, i, route, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateRoute(name string, index int, route Route, seen map[string]bool) error {
	method := strings.ToUpper(route.Method)
	if !mockMethods[method] {
		return fmt.Errorf("endpoint %q mock route %d declares unknown method %q", name, index, route.Method)
	}
	if !strings.HasPrefix(route.Path, "/") {
		return fmt.Errorf("endpoint %q mock route %d declares path %q; want a path beginning with /", name, index, route.Path)
	}
	if len(route.Responses) == 0 {
		return fmt.Errorf("endpoint %q mock route %s %s declares no responses", name, method, route.Path)
	}
	if err := validateResponses(name, method, route); err != nil {
		return err
	}
	key := routeKey(method, route.Path)
	if seen[key] {
		return fmt.Errorf("endpoint %q mock config declares duplicate route %q", name, key)
	}
	seen[key] = true
	return nil
}

func validateResponses(name, method string, route Route) error {
	for j, response := range route.Responses {
		if response.Status < 100 || response.Status > 599 {
			return fmt.Errorf("endpoint %q mock route %s %s response %d declares status %d; want 100-599",
				name, method, route.Path, j, response.Status)
		}
	}
	return nil
}
