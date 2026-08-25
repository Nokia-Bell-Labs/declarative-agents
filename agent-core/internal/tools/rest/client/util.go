// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

const (
	initSend   = "rest_client_send"
	initAwait  = "rest_client_await"
	initInvoke = "rest_client_invoke"

	authNone        = "none"
	authBasic       = "basic"
	authBearer      = "bearer"
	authHeaderToken = "header_token"
	authQueryToken  = "query_token"

	redirectNone      = "none"
	redirectSameHost  = "same_host"
	redirectAllowlist = "allowlist"

	defaultAwaitTimeout = 30 * time.Second
)

var (
	bodyParamPattern = regexp.MustCompile(`params\.([A-Za-z_][A-Za-z0-9_]*)`)
	pathParamPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?:\.\.\.)?\}`)
)

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

func parseDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

func jsonOutput(value map[string]interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}
