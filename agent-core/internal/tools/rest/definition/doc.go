// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package definition is the REST YAML model and loader (srd027, rest-tool-format).
//
// It has no rest-internal imports. LoadDefinition and ParseDefinition decode and
// compile OpenAPI imports; they do not validate. The parent rest package
// composes parse with rest/validation so Collection.Add still receives a
// checked definition. Validation imports this package; this package does not
// import validation, which keeps the DAG acyclic.
package definition
