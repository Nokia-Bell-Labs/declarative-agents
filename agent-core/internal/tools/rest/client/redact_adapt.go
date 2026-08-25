// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/redact"
)

func redactSource(def ClientOperationDefinition) redact.Source {
	mappings := make(map[string]redact.Mapping, len(def.ResponseMappings))
	for name, mapping := range def.ResponseMappings {
		mappings[name] = toRedactMapping(mapping)
	}
	return redact.Source{
		Auth:             redact.Auth{Header: def.Auth.Header, Query: def.Auth.Query},
		ResponseMappings: mappings,
		Operation:        toRedactMapping(def.Operation.Response),
		OperationRef:     def.Operation.ResponseRef,
	}
}

func toRedactStatus(mapping StatusMapping) redact.Status {
	return redact.Status{ResponseRef: mapping.ResponseRef, Response: toRedactMapping(mapping.Response)}
}

func toRedactMapping(mapping ResponseMapping) redact.Mapping {
	return redact.Mapping{
		Schema: mapping.Schema, Output: mapping.Output, Redact: mapping.Redact,
		ResourceID: mapping.ResourceID, RequestID: mapping.RequestID,
	}
}

func fromRedactMapping(mapping redact.Mapping) ResponseMapping {
	return ResponseMapping{
		Schema: mapping.Schema, Output: mapping.Output, Redact: mapping.Redact,
		ResourceID: mapping.ResourceID, RequestID: mapping.RequestID,
	}
}

func clientRedactionSelectors(def ClientOperationDefinition, mapping StatusMapping) []string {
	return redact.Selectors(redactSource(def), toRedactStatus(mapping))
}

func clientOutputRedaction(
	def ClientOperationDefinition,
	mapping StatusMapping,
	selectors []string,
) core.OutputRedaction {
	return redact.OutputRedaction(redactSource(def), toRedactStatus(mapping), selectors)
}

func redactClientOutput(output map[string]interface{}, selectors []string) {
	redact.ClientOutput(output, selectors)
}

func redactClientDerivedOutput(
	output map[string]interface{},
	responseMap ResponseMapping,
	selectors []string,
) {
	redact.DerivedOutput(output, toRedactMapping(responseMap), selectors)
}

// ResolvedResponseMapping picks the response mapping for one status mapping.
func ResolvedResponseMapping(def ClientOperationDefinition, mapping StatusMapping) ResponseMapping {
	return fromRedactMapping(redact.ResolveMapping(redactSource(def), toRedactStatus(mapping)))
}

func resolvedResponseMapping(def ClientOperationDefinition, mapping StatusMapping) ResponseMapping {
	return ResolvedResponseMapping(def, mapping)
}
