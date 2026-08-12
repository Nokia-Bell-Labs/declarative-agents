// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"errors"
	"fmt"
	"os"
)

type credentialResolutionError struct {
	ref string
}

func (e credentialResolutionError) Error() string {
	return fmt.Sprintf("credential ref %q is not resolved", e.ref)
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

func (EnvironmentCredentials) ResolveCredential(ref string) (string, error) {
	//nolint:forbidigo // srd028 R2.6/R9.3: trusted auth config selects this resolver and names only a credential reference; secret values stay outside config and are redacted from diagnostics.
	value, ok := os.LookupEnv(ref)
	if !ok || value == "" {
		return "", credentialResolutionError{ref: ref}
	}
	return value, nil
}

func isCredentialResolutionError(err error) bool {
	var target credentialResolutionError
	return errors.As(err, &target)
}
