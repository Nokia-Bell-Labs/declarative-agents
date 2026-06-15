// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type credentialResolutionError struct {
	ref string
}

func (e credentialResolutionError) Error() string {
	return fmt.Sprintf("credential ref %q is not resolved", e.ref)
}

func buildClientRequest(
	def ClientOperationDefinition,
	input map[string]interface{},
	resolver CredentialResolver,
) (*http.Request, error) {
	params, err := normalizeRuntimeParams(input, def.Operation.Params)
	if err != nil {
		return nil, err
	}
	endpoint, err := renderURL(def, params)
	if err != nil {
		return nil, err
	}
	body, err := renderRequestBody(def.Operation, params, def.Limits.MaxRequestBytes)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(def.Operation.Method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return renderRequestBody(def.Operation, params, def.Limits.MaxRequestBytes)
	}
	applyHeaders(req, def.Operation.Params.Headers, params)
	return req, applyAuth(req, def.Auth, resolver)
}

func normalizeRuntimeParams(input map[string]interface{}, binding RequestBinding) (map[string]interface{}, error) {
	if err := ValidateRuntimeInput(input); err != nil {
		return nil, err
	}
	params := input
	if nested, ok := input["params"].(map[string]interface{}); ok {
		params = nested
	}
	if err := validateDeclaredRuntimeParams(params, binding); err != nil {
		return nil, err
	}
	return params, validateBodySchema(binding.BodySchema, params)
}

func validateDeclaredRuntimeParams(params map[string]interface{}, binding RequestBinding) error {
	declared := declaredParamNames(binding)
	for name := range params {
		if name == "body" {
			continue
		}
		if !declared[name] {
			return fmt.Errorf("runtime param %q is not declared", name)
		}
	}
	for name := range binding.Path {
		if _, ok := params[name]; !ok {
			return fmt.Errorf("path param %q is required", name)
		}
	}
	return nil
}

func declaredParamNames(binding RequestBinding) map[string]bool {
	names := map[string]bool{}
	for name := range binding.Path {
		names[name] = true
	}
	for name := range binding.Query {
		names[name] = true
	}
	for name := range binding.Headers {
		names[name] = true
	}
	for name := range schemaProperties(binding.BodySchema) {
		names[name] = true
	}
	return names
}

func renderURL(def ClientOperationDefinition, params map[string]interface{}) (string, error) {
	base, err := url.Parse(def.Client.BaseURL)
	if err != nil {
		return "", err
	}
	path := renderPath(def.Operation.Path, params)
	rel, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	endpoint := base.ResolveReference(rel)
	query := endpoint.Query()
	for name := range def.Operation.Params.Query {
		query.Set(name, fmt.Sprint(params[name]))
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), validateNetwork(endpoint, def.Limits.Network)
}

func renderPath(path string, params map[string]interface{}) string {
	for _, match := range pathParamPattern.FindAllStringSubmatch(path, -1) {
		path = strings.ReplaceAll(path, match[0], escapedPathParam(match[0], fmt.Sprint(params[match[1]])))
	}
	return path
}

func escapedPathParam(token, value string) string {
	if !strings.HasSuffix(token, "...}") {
		return url.PathEscape(value)
	}
	parts := strings.Split(value, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func renderRequestBody(operation Operation, params map[string]interface{}, maxBytes int) (io.ReadCloser, error) {
	if len(operation.Body) == 0 {
		return http.NoBody, nil
	}
	rendered, err := renderTemplateValue(operation.Body, params)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(rendered)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return nil, fmt.Errorf("request body exceeds max_request_bytes %d", maxBytes)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func renderTemplateValue(value interface{}, params map[string]interface{}) (interface{}, error) {
	switch typed := value.(type) {
	case string:
		return renderTemplateString(typed, params)
	case map[string]interface{}:
		return renderTemplateMap(typed, params)
	case []interface{}:
		return renderTemplateSlice(typed, params)
	default:
		return typed, nil
	}
}

func renderTemplateString(value string, params map[string]interface{}) (interface{}, error) {
	matches := bodyParamPattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 1 && value == templateToken(matches[0][0]) {
		return params[matches[0][1]], nil
	}
	for _, match := range matches {
		value = strings.ReplaceAll(value, templateToken(match[0]), fmt.Sprint(params[match[1]]))
	}
	return value, nil
}

func templateToken(field string) string {
	return "{{ " + field + " }}"
}

func renderTemplateMap(values map[string]interface{}, params map[string]interface{}) (map[string]interface{}, error) {
	rendered := map[string]interface{}{}
	for key, value := range values {
		item, err := renderTemplateValue(value, params)
		if err != nil {
			return nil, err
		}
		rendered[key] = item
	}
	return rendered, nil
}

func renderTemplateSlice(values []interface{}, params map[string]interface{}) ([]interface{}, error) {
	rendered := make([]interface{}, 0, len(values))
	for _, value := range values {
		item, err := renderTemplateValue(value, params)
		if err != nil {
			return nil, err
		}
		rendered = append(rendered, item)
	}
	return rendered, nil
}

func applyHeaders(req *http.Request, declared map[string]interface{}, params map[string]interface{}) {
	for name := range declared {
		req.Header.Set(name, fmt.Sprint(params[name]))
	}
	if req.Body != http.NoBody {
		req.Header.Set("Content-Type", "application/json")
	}
}

func applyAuth(req *http.Request, auth AuthProfile, resolver CredentialResolver) error {
	switch auth.Type {
	case "", authNone:
		return nil
	case authBearer:
		token, err := resolveCredential(resolver, auth.TokenRef)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", bearerValue(auth.Scheme, token))
	case authHeaderToken:
		token, err := resolveCredential(resolver, auth.TokenRef)
		if err != nil {
			return err
		}
		req.Header.Set(auth.Header, token)
	case authQueryToken:
		token, err := resolveCredential(resolver, auth.TokenRef)
		if err != nil {
			return err
		}
		query := req.URL.Query()
		query.Set(auth.Query, token)
		req.URL.RawQuery = query.Encode()
	case authBasic:
		username, err := resolveCredential(resolver, auth.UsernameRef)
		if err != nil {
			return err
		}
		password, err := resolveCredential(resolver, auth.PasswordRef)
		if err != nil {
			return err
		}
		req.SetBasicAuth(username, password)
	}
	return nil
}

func bearerValue(scheme, token string) string {
	if scheme == "" {
		scheme = "Bearer"
	}
	return scheme + " " + token
}

func resolveCredential(resolver CredentialResolver, ref string) (string, error) {
	if ref == "" || resolver == nil {
		return "", credentialResolutionError{ref: ref}
	}
	return resolver.ResolveCredential(ref)
}

func (c StaticCredentials) ResolveCredential(ref string) (string, error) {
	value, ok := c[ref]
	if !ok {
		return "", credentialResolutionError{ref: ref}
	}
	return value, nil
}

func (EmptyCredentialResolver) ResolveCredential(ref string) (string, error) {
	return "", credentialResolutionError{ref: ref}
}

func isCredentialResolutionError(err error) bool {
	var target credentialResolutionError
	return errors.As(err, &target)
}

func validateNetwork(endpoint *url.URL, policy NetworkPolicy) error {
	if len(policy.Schemes) > 0 && !stringIn(endpoint.Scheme, policy.Schemes) {
		return fmt.Errorf("scheme %q is not allowed", endpoint.Scheme)
	}
	host := endpoint.Hostname()
	if len(policy.Hosts) > 0 && !stringIn(host, policy.Hosts) {
		return fmt.Errorf("host %q is not allowed", host)
	}
	return validatePort(endpoint, policy)
}

func validatePort(endpoint *url.URL, policy NetworkPolicy) error {
	if len(policy.Ports) == 0 {
		return nil
	}
	port := endpoint.Port()
	if port == "" {
		port = defaultPort(endpoint.Scheme)
	}
	for _, allowed := range policy.Ports {
		if fmt.Sprint(allowed) == port {
			return nil
		}
	}
	return fmt.Errorf("port %q is not allowed", port)
}

func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

func stringIn(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}

func schemaProperties(schema map[string]interface{}) map[string]interface{} {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return nil
	}
	return props
}
