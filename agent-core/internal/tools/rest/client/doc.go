// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package client is the REST outbound execution path (srd028).
//
// The parent imports this package; this package does not import rest. YAML
// model types live in rest/definition and are aliased here so execution can
// name them without a cycle. Helpers the parent reuses: RetryDelay, PortInRange,
// ResolveResultSelector, SchemaProperties, BearerValue, ValidateBodySchema,
// DeclaredParamNames, ValidateRuntimeInput.
package client
