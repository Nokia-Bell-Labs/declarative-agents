// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package rest is the REST tool parent: collection/resolution, the inbound
// server runtime, and factories (srd027, srd028, srd029, srd038).
//
// YAML model and loading live in rest/definition; ValidateDefinition lives in
// rest/validation. This package imports both and composes parse+validate for
// LoadDefinition. Leaf packages (client, credentials, redact, monitor, mock,
// servercmd) do not import this parent.
package rest
