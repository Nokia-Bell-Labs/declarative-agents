// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package validation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestResolveTestEvidenceBuilderReportsInventoryBuildFailure(t *testing.T) {
	t.Parallel()
	vs := evidenceState(claimTestClaimedSuites())
	cmd := (&ResolveTestEvidenceBuilder{
		VS: vs, ModuleFrom: moduleSel, PackagesFrom: packagesSel, TestsFrom: testsSel,
	}).Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
		"go_module":         outputPayloadStr(evidenceModule),
		"go_packages":       outputPayloadStr(evidencePackage + "\n"),
		"go_test_inventory": outputPayloadStr(mustInventoryFailureStream(evidencePackage)),
	}))

	result := cmd.Execute()

	require.Equal(t, core.ValidationFailed, result.Signal, result.Output)
	require.NoError(t, result.Err)
	require.NotNil(t, vs.TestInventory)
	require.Len(t, vs.Findings, 1, "inventory failure must not cascade into missing-claim findings")
	require.Contains(t, findingMessages(vs.Findings), evidencePackage)
	require.Contains(t, findingMessages(vs.Findings), "no required module provides package example.test/missing")
}

func TestResolveTestEvidenceBuilderKeepsNonGoInventoryFailureAsCommandError(t *testing.T) {
	t.Parallel()
	vs := evidenceState(claimTestClaimedSuites())
	cmd := (&ResolveTestEvidenceBuilder{
		VS: vs, ModuleFrom: moduleSel, PackagesFrom: packagesSel, TestsFrom: testsSel,
	}).Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
		"go_module":         outputPayloadStr(evidenceModule),
		"go_packages":       outputPayloadStr(evidencePackage + "\n"),
		"go_test_inventory": outputPayloadStr("fork/exec go: resource temporarily unavailable"),
	}))

	result := cmd.Execute()

	require.Equal(t, core.CommandError, result.Signal, result.Output)
	require.Error(t, result.Err)
	require.Contains(t, result.Output, "no Go JSON package events")
	require.Nil(t, vs.TestInventory)
	require.Empty(t, vs.Findings)
}

func mustInventoryFailureStream(pkg string) string {
	var b strings.Builder
	for _, event := range []map[string]string{
		{"Action": "start", "Package": pkg},
		{"Action": "output", "Package": pkg, "Output": "FAIL\t" + pkg + " [setup failed]\n"},
		{
			"Action": "output", "Package": pkg,
			"Output": "no required module provides package example.test/missing\n",
		},
		{"Action": "fail", "Package": pkg},
	} {
		data, _ := json.Marshal(event)
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}
