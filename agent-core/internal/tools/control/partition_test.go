// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestPartitionPreservesOrderAndCounts(t *testing.T) {
	cmd := PartitionBuilder{
		ToolName: "partition", Items: "$from(values).items", Field: "meta.score",
		Op: OpGt, Right: "6", Satisfied: "Partitioned",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(stubView{
		"values": `{"items":[{"id":"a","meta":{"score":"10"}},{"id":"b","meta":{"score":"2"}},{"id":"c","meta":{"score":"8"}}]}`,
	})

	res := cmd.Execute()
	require.NoError(t, res.Err)
	require.Equal(t, core.Signal("Partitioned"), res.Signal)
	var output struct {
		Matched        []map[string]interface{} `json:"matched"`
		Unmatched      []map[string]interface{} `json:"unmatched"`
		MatchedCount   int                      `json:"matched_count"`
		UnmatchedCount int                      `json:"unmatched_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &output))
	require.Equal(t, []interface{}{"a", "c"}, []interface{}{output.Matched[0]["id"], output.Matched[1]["id"]})
	require.Equal(t, "b", output.Unmatched[0]["id"])
	require.Equal(t, 2, output.MatchedCount)
	require.Equal(t, 1, output.UnmatchedCount)
}

func TestPartitionUnresolvedSelectorIsCommandError(t *testing.T) {
	cmd := PartitionBuilder{
		ToolName: "partition", Items: "$from(missing).items", Field: "value",
		Op: OpEq, Right: "x", OperandType: OperandString, Satisfied: "Partitioned",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(stubView{})
	res := cmd.Execute()
	require.Equal(t, core.CommandError, res.Signal)
	var unresolved *core.UnresolvedLabelError
	require.True(t, errors.As(res.Err, &unresolved))
}

func TestValidatePartitionConfigRejectsMalformedConfig(t *testing.T) {
	require.Error(t, ValidatePartitionConfig("p", "$.items", "value", OpEq, "x", OperandString, "Done"))
	require.Error(t, ValidatePartitionConfig("p", "$from(x).items", "", OpEq, "x", OperandString, "Done"))
	require.Error(t, ValidatePartitionConfig("p", "$from(x).items", "value", "unknown", "x", OperandString, "Done"))
	require.Error(t, ValidatePartitionConfig("p", "$from(x).items", "value", OpEq, "", OperandString, "Done"))
	require.Error(t, ValidatePartitionConfig("p", "$from(x).items", "value", OpEq, "x", "boolean", "Done"))
	require.Error(t, ValidatePartitionConfig("p", "$from(x).items", "value", OpEq, "x", OperandString, ""))
	require.NoError(t, ValidatePartitionConfig(
		"p", "$from(x).items", "value", OpEq, "$from(y).target", OperandString, "Done",
	))
}
