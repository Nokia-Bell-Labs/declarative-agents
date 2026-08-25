// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package client is the REST outbound execution path (srd028).
//
// The parent imports this package; this package does not import rest. YAML
// model types the server also uses live here and are aliased from rest so
// loading and validation stay in the parent until GH-1823.
//
// Helpers the parent reuses: RetryDelay, PortInRange, ResolveResultSelector,
// SchemaProperties, BearerValue, ValidateBodySchema, DeclaredParamNames,
// ValidateRuntimeInput. parseDuration, jsonOutput, path/body param patterns,
// and the forbidden-authority field set are duplicated here — the server
// runtime and load validation need them in rest, and this package cannot
// import the parent.
package client
