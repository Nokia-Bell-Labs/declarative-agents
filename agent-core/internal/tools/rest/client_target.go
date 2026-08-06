// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"unicode"

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
	selected, err := selectedBaseURL(operation, view)
	if err != nil {
		return "", true, err
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

// selectedBaseURL resolves the operation's runtime-selected base URL. A
// base_url_selector resolves one whole absolute URL (srd028 R14.1); a
// base_url_host_selector resolves a bare host or IP that is composed with the
// operation's declared scheme and port (srd028 R14.6).
func selectedBaseURL(operation Operation, view core.CommandStateView) (string, error) {
	if operation.BaseURLHostSelector == "" {
		return selectedSelectorString("base_url_selector", operation.BaseURLSelector, "URL", view)
	}
	host, err := selectedSelectorString("base_url_host_selector", operation.BaseURLHostSelector, "host", view)
	if err != nil {
		return "", err
	}
	if err := validateSelectedHost(host); err != nil {
		return "", err
	}
	return composedBaseURL(operation, host), nil
}

// selectedSelectorString resolves one command-state selector to a nonempty
// trimmed string, naming the operation field so a diagnostic points at the
// authored configuration.
func selectedSelectorString(field, selector, subject string, view core.CommandStateView) (string, error) {
	value, err := core.ResolveFromSelector(view, selector)
	if err != nil {
		return "", err
	}
	selected, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s %q resolved to %T, want string", field, selector, value)
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", fmt.Errorf("%s %q resolved to an empty %s", field, selector, subject)
	}
	return selected, nil
}

// composedBaseURL builds scheme://host[:port] from an already validated bare
// host. The scheme and port come from trusted operation config, never from the
// selected value, so a discovered host cannot widen transport authority.
func composedBaseURL(operation Operation, host string) string {
	scheme := operation.BaseURLScheme
	if scheme == "" {
		scheme = "http"
	}
	if port := strings.TrimSpace(operation.BaseURLPort); port != "" {
		return scheme + "://" + net.JoinHostPort(host, port)
	}
	return scheme + "://" + bracketedHost(host)
}

// bracketedHost wraps an IPv6 literal so it parses as a URL authority.
func bracketedHost(host string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

// validateSelectedHost rejects a resolved value that is not a bare host or IP,
// so it cannot smuggle a scheme, credential, port, path, or query into the
// composed authority (srd028 R14.3).
func validateSelectedHost(host string) error {
	if strings.ContainsAny(host, "/@?#\\") {
		return fmt.Errorf("selected host %q must be a bare host or IP without scheme, credentials, or path", host)
	}
	for _, r := range host {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("selected host %q must not contain whitespace or control characters", host)
		}
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	if strings.Contains(host, ":") {
		return fmt.Errorf("selected host %q must not carry a port; declare base_url_port", host)
	}
	return nil
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
