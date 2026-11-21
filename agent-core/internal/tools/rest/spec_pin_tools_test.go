// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"net/http"
	"testing"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
)

// Pinning tests for application-dsl-spec 1.0 statements whose REST
// validation rules no other test exercised (R-REST-005, R-REST-008).

func TestSpecPinIrreversibleOperationRequiresConfirmation(t *testing.T) {
	t.Parallel()
	operation := validWriteOperation()
	operation.Reversibility = restdef.Reversibility{Classification: "irreversible"}
	operation.Compensation = nil
	def := baseDefinition()
	def.Clients["github"].Operations["mutate"] = operation

	require.ErrorContains(t, ValidateDefinition(def), "irreversible without confirmation")

	operation.Reversibility.RequiresConfirmation = true
	def.Clients["github"].Operations["mutate"] = operation
	require.NoError(t, ValidateDefinition(def))
}

func TestSpecPinMachineRequestRequiresTargetAndTimeout(t *testing.T) {
	t.Parallel()
	mapping := map[string]restdef.MachineResponseMapping{"Done": {Status: http.StatusOK}}
	endpoint := func(request restdef.MachineRequest) restdef.Endpoint {
		return restdef.Endpoint{Method: "POST", Path: "/docs", Binding: bindingMachineRequest, MachineRequest: request}
	}

	err := validateMachineRequestEndpoint("docs", endpoint(restdef.MachineRequest{
		Timeout: "2s", Response: restdef.MachineRequestResponse{TerminalSignals: mapping},
	}))
	require.ErrorContains(t, err, "requires profile, machine, or machine spec")

	err = validateMachineRequestEndpoint("docs", endpoint(restdef.MachineRequest{
		Profile: "request/profile.yaml", Response: restdef.MachineRequestResponse{TerminalSignals: mapping},
	}))
	require.ErrorContains(t, err, "requires timeout")

	require.NoError(t, validateMachineRequestEndpoint("docs", endpoint(restdef.MachineRequest{
		Profile: "request/profile.yaml", Timeout: "2s",
		Response: restdef.MachineRequestResponse{TerminalSignals: mapping},
	})))
}
