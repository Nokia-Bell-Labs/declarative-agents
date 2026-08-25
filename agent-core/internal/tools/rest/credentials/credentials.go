// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package credentials resolves trusted REST credential references (srd028).
// The parent re-exports the types under their historical names so cmd/agent
// and lifecycle keep compiling until the final REST split (GH-1823).
package credentials

import (
	"errors"
	"fmt"
	"os"
)

// Resolver resolves trusted runtime credentials by reference.
type Resolver interface {
	ResolveCredential(ref string) (string, error)
}

// Static resolves credentials from an in-memory trusted map.
type Static map[string]string

// Empty resolves no credential references.
type Empty struct{}

// Environment resolves trusted references from the process environment.
// Definitions carry reference names, never inline secret values.
type Environment struct{}

// ResolutionError is returned when a credential reference cannot be resolved.
type ResolutionError struct {
	Ref string
}

func (e ResolutionError) Error() string {
	return fmt.Sprintf("credential ref %q is not resolved", e.Ref)
}

// Resolve looks up one credential reference.
func Resolve(resolver Resolver, ref string) (string, error) {
	if ref == "" || resolver == nil {
		return "", ResolutionError{Ref: ref}
	}
	return resolver.ResolveCredential(ref)
}

func (c Static) ResolveCredential(ref string) (string, error) {
	value, ok := c[ref]
	if !ok {
		return "", ResolutionError{Ref: ref}
	}
	return value, nil
}

func (Empty) ResolveCredential(ref string) (string, error) {
	return "", ResolutionError{Ref: ref}
}

func (Environment) ResolveCredential(ref string) (string, error) {
	//nolint:forbidigo // srd028 R2.6/R9.3: trusted auth config selects this resolver and names only a credential reference; secret values stay outside config and are redacted from diagnostics.
	value, ok := os.LookupEnv(ref)
	if !ok || value == "" {
		return "", ResolutionError{Ref: ref}
	}
	return value, nil
}

// IsResolutionError reports a missing or empty credential reference.
func IsResolutionError(err error) bool {
	var target ResolutionError
	return errors.As(err, &target)
}
