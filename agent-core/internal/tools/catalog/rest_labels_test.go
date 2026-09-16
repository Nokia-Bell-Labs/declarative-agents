// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// stubOperations answers for one configured operation and nothing else, which
// is how an unresolvable operation leaves a label untyped.
type stubOperations struct {
	operation string
	response  RESTClientResponse
}

func (s stubOperations) ClientResponse(config map[string]interface{}) (RESTClientResponse, bool) {
	if name, _ := config["operation"].(string); name != s.operation {
		return RESTClientResponse{}, false
	}
	return s.response, true
}

func listPods() stubOperations {
	return stubOperations{
		operation: "list_pods",
		response:  RESTClientResponse{Mapped: []string{"pods"}, Carried: []string{"namespace"}},
	}
}

func discoverPodsTool() ToolDef {
	return ToolDef{
		Name: "discover", Init: "rest_client_get",
		Config: map[string]interface{}{"rest_ref": "kube_api", "operation": "list_pods"},
	}
}

// restMachine publishes the REST word's result under a label a later word reads.
func restMachine(selector string) (core.MachineSpec, []ToolDef) {
	spec := core.MachineSpec{Transitions: []core.TransitionSpec{
		{State: "S0", Signal: "Seed", Next: "S1", Action: "discover", Label: "pods_result"},
		{State: "S1", Signal: "ToolDone", Next: "Done", Action: "report"},
	}}
	return spec, []ToolDef{
		discoverPodsTool(),
		{Name: "report", Config: map[string]interface{}{"source": selector}},
	}
}

// TestRESTClientLabelResolvesTheMappedField is GH-1969's finding as a test: a
// REST word publishes the envelope, so mapped.pods resolves where the word's
// own output.schema would have rejected it.
func TestRESTClientLabelResolvesTheMappedField(t *testing.T) {
	t.Parallel()
	spec, defs := restMachine("$from(pods_result).mapped.pods")
	require.Empty(t, ValidateSelectorPaths(spec, defs, nil, listPods()))
}

func TestRESTClientLabelRejectsAFieldTheMappingDoesNotPublish(t *testing.T) {
	t.Parallel()
	spec, defs := restMachine("$from(pods_result).mapped.podz")

	diagnostics := ValidateSelectorPaths(spec, defs, nil, listPods())

	require.Len(t, diagnostics, 1)
	require.Equal(t, core.DiagnosticSelectorPathMismatch, diagnostics[0].Code)
	require.Contains(t, diagnostics[0].Message, `has no field "podz"`)
	require.Contains(t, diagnostics[0].Message, `closest declared field is "pods"`)
}

// TestRESTClientLabelCarriesTheUniformEnvelope covers the twelve fields the
// runtime fills the same way for every REST word, which a signature would have
// restated on each of them.
func TestRESTClientLabelCarriesTheUniformEnvelope(t *testing.T) {
	t.Parallel()
	for _, selector := range []string{
		"$from(pods_result).status",
		"$from(pods_result).headers",
		"$from(pods_result).retry_count",
		"$from(pods_result).domain_error_code",
		"$from(pods_result).selected_authority",
		"$from(pods_result).body.anything",
		"$from(pods_result).carried.namespace",
	} {
		spec, defs := restMachine(selector)
		require.Emptyf(t, ValidateSelectorPaths(spec, defs, nil, listPods()),
			"selector %s", selector)
	}
}

func TestRESTClientLabelRejectsAFieldTheEnvelopeLacks(t *testing.T) {
	t.Parallel()
	spec, defs := restMachine("$from(pods_result).stats")

	diagnostics := ValidateSelectorPaths(spec, defs, nil, listPods())

	require.Len(t, diagnostics, 1)
	require.Contains(t, diagnostics[0].Message, `has no field "stats"`)
}

// TestUnresolvableOperationLeavesTheLabelUntyped keeps deriving gradual: a word
// whose operation does not resolve reports nothing here, and the load reports
// the unresolvable operation itself.
func TestUnresolvableOperationLeavesTheLabelUntyped(t *testing.T) {
	t.Parallel()
	spec, defs := restMachine("$from(pods_result).mapped.anything")
	operations := stubOperations{operation: "a_different_operation"}

	require.Empty(t, ValidateSelectorPaths(spec, defs, nil, operations))
}

// TestNonRESTWordKeepsItsSignatureType keeps the derivation scoped: a word that
// is not a REST client takes the type its signature states, not an envelope.
func TestNonRESTWordKeepsItsSignatureType(t *testing.T) {
	t.Parallel()
	spec, defs := restMachine("$from(pods_result).mapped.pods")
	defs[0].Init = "compose"

	require.Empty(t, ValidateSelectorPaths(spec, defs, nil, listPods()),
		"an unsigned non-REST word publishes an untyped label")
}

// TestRESTLabelIsNotDerivedWithoutAResolver keeps a caller that supplies none
// working as it did, which is how every existing test and the point machines
// still load.
func TestRESTLabelIsNotDerivedWithoutAResolver(t *testing.T) {
	t.Parallel()
	spec, defs := restMachine("$from(pods_result).mapped.anything")

	require.Empty(t, ValidateSelectorPaths(spec, defs, nil, nil))
}
