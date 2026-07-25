// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type targetResolutionError struct {
	err error
}

func (e targetResolutionError) Error() string { return e.err.Error() }
func (e targetResolutionError) Unwrap() error { return e.err }

func resolveOperationBaseURL(operation Operation, configured string, view core.CommandStateView) (string, bool, error) {
	if operation.BaseURLSource == "" {
		return configured, false, nil
	}
	if view == nil {
		return "", true, fmt.Errorf("base_url_source %s requires a configured command-state store", operation.BaseURLSource)
	}
	value, err := core.ResolveFromSelector(view, operation.BaseURLSelector)
	if err != nil {
		return "", true, err
	}
	selected, ok := value.(string)
	if !ok {
		return "", true, fmt.Errorf("base_url_selector %q resolved to %T, want string", operation.BaseURLSelector, value)
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", true, fmt.Errorf("base_url_selector %q resolved to an empty URL", operation.BaseURLSelector)
	}
	parsed, err := url.Parse(selected)
	if err != nil {
		return "", true, fmt.Errorf("parse selected base URL: %w", err)
	}
	if err := validateSelectedBaseURL(parsed); err != nil {
		return "", true, err
	}
	return parsed.String(), true, nil
}

func validateSelectedBaseURL(base *url.URL) error {
	if base == nil || base.Scheme == "" {
		return fmt.Errorf("selected base URL must be an absolute HTTP URL")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return fmt.Errorf("selected base URL scheme %q is not allowed", base.Scheme)
	}
	if !base.IsAbs() || base.Host == "" {
		return fmt.Errorf("selected base URL must be an absolute HTTP URL")
	}
	if base.User != nil {
		return fmt.Errorf("selected base URL must not contain user information")
	}
	return nil
}

func validateSelectedEndpoint(def ClientOperationDefinition, endpoint *url.URL) error {
	if err := validateSelectedBaseURL(endpoint); err != nil {
		return err
	}
	if def.Auth.Type == "" || def.Auth.Type == authNone || def.Operation.AllowSelectedAuth {
		return nil
	}
	configured, err := url.Parse(def.Client.BaseURL)
	if err != nil {
		return fmt.Errorf("parse configured base URL for credential scope: %w", err)
	}
	if canonicalAuthority(configured) != canonicalAuthority(endpoint) {
		return fmt.Errorf(
			"selected authority %q differs from credential scope %q; set allow_auth_on_selected_authority only for a trusted operation",
			canonicalAuthority(endpoint),
			canonicalAuthority(configured),
		)
	}
	return nil
}

func canonicalAuthority(endpoint *url.URL) string {
	if endpoint == nil {
		return ""
	}
	port := endpoint.Port()
	if port == "" {
		port = defaultPort(strings.ToLower(endpoint.Scheme))
	}
	return strings.ToLower(endpoint.Scheme) + "://" + strings.ToLower(endpoint.Hostname()) + ":" + port
}
