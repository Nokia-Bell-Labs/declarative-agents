// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

type lifecycleAuthError struct {
	status int
	err    error
}

func (e lifecycleAuthError) Error() string { return e.err.Error() }

func authorizeLifecycleRequest(
	req *http.Request, def ServerDefinition, authRef string,
) error {
	if authRef == "" {
		if remoteIsLoopback(req.RemoteAddr) {
			return nil
		}
		return lifecycleAuthError{
			status: http.StatusForbidden,
			err:    fmt.Errorf("unauthenticated lifecycle control is loopback-only"),
		}
	}
	auth, ok := def.Auth[authRef]
	if !ok {
		return lifecycleAuthError{
			status: http.StatusInternalServerError,
			err:    fmt.Errorf("lifecycle auth profile %q is not defined", authRef),
		}
	}
	return authorizeWithProfile(req, auth, def.Credentials)
}

// withoutLifecycleCredential removes verified secret material before ordinary
// request projection, validation, redaction, queueing, and tracing.
func withoutLifecycleCredential(
	req *http.Request, def ServerDefinition, authRef string,
) *http.Request {
	if authRef == "" {
		return req
	}
	auth, ok := def.Auth[authRef]
	if !ok {
		return req
	}
	sanitized := req.Clone(req.Context())
	sanitized.Header = req.Header.Clone()
	sanitized.URL = cloneURL(req)
	switch auth.Type {
	case authBearer, authBasic:
		sanitized.Header.Del("Authorization")
	case authHeaderToken:
		sanitized.Header.Del(auth.Header)
	case authQueryToken:
		query := sanitized.URL.Query()
		query.Del(auth.Query)
		sanitized.URL.RawQuery = query.Encode()
	}
	return sanitized
}

func cloneURL(req *http.Request) *url.URL {
	if req.URL == nil {
		return &url.URL{}
	}
	clone := *req.URL
	return &clone
}

func authorizeWithProfile(
	req *http.Request, auth restdef.AuthProfile, resolver credentials.Resolver,
) error {
	switch auth.Type {
	case "", authNone:
		if remoteIsLoopback(req.RemoteAddr) {
			return nil
		}
		return lifecycleAuthError{status: http.StatusForbidden,
			err: fmt.Errorf("lifecycle auth profile type none is loopback-only")}
	case authBearer:
		token, err := resolveCredential(resolver, auth.TokenRef)
		return compareLifecycleCredential(req.Header.Get("Authorization"), bearerValue(auth.Scheme, token), err)
	case authHeaderToken:
		token, err := resolveCredential(resolver, auth.TokenRef)
		return compareLifecycleCredential(req.Header.Get(auth.Header), token, err)
	case authQueryToken:
		token, err := resolveCredential(resolver, auth.TokenRef)
		return compareLifecycleCredential(req.URL.Query().Get(auth.Query), token, err)
	case authBasic:
		return authorizeLifecycleBasic(req, auth, resolver)
	default:
		return lifecycleAuthError{
			status: http.StatusInternalServerError,
			err:    fmt.Errorf("unsupported lifecycle auth type %q", auth.Type),
		}
	}
}

func authorizeLifecycleBasic(
	req *http.Request, auth restdef.AuthProfile, resolver credentials.Resolver,
) error {
	username, userErr := resolveCredential(resolver, auth.UsernameRef)
	password, passErr := resolveCredential(resolver, auth.PasswordRef)
	if userErr != nil || passErr != nil {
		return lifecycleCredentialConfigError(userErr, passErr)
	}
	gotUser, gotPassword, ok := req.BasicAuth()
	if !ok || !secureEqual(gotUser, username) || !secureEqual(gotPassword, password) {
		return lifecycleUnauthorized()
	}
	return nil
}

func compareLifecycleCredential(actual, expected string, resolveErr error) error {
	if resolveErr != nil {
		return lifecycleCredentialConfigError(resolveErr)
	}
	if !secureEqual(actual, expected) {
		return lifecycleUnauthorized()
	}
	return nil
}

func lifecycleCredentialConfigError(errs ...error) error {
	return lifecycleAuthError{
		status: http.StatusInternalServerError,
		err:    fmt.Errorf("resolve lifecycle credential: %v", errs),
	}
}

func lifecycleUnauthorized() error {
	return lifecycleAuthError{
		status: http.StatusUnauthorized, err: fmt.Errorf("lifecycle authentication failed"),
	}
}

func secureEqual(actual, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func remoteIsLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
