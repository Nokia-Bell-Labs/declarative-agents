// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "fmt"

var forbiddenRuntimeAuthorityFields = map[string]bool{
	"auth":            true,
	"auth_ref":        true,
	"base_url":        true,
	"host":            true,
	"method":          true,
	"redirect":        true,
	"redirect_policy": true,
	"url":             true,
}

// ValidateRuntimeInput rejects transport authority supplied at runtime.
func ValidateRuntimeInput(input map[string]interface{}) error {
	for name := range input {
		if forbiddenRuntimeAuthorityFields[name] {
			return fmt.Errorf("runtime input field %q cannot set REST authority", name)
		}
	}
	params, ok := input["params"].(map[string]interface{})
	if !ok {
		return nil
	}
	for name := range params {
		if forbiddenRuntimeAuthorityFields[name] {
			return fmt.Errorf("runtime input params.%s cannot set REST authority", name)
		}
	}
	return nil
}
