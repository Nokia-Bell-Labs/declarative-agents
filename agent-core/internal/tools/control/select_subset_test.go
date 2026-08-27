// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package control

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/compose"
)

func runSelectSubset(t *testing.T, candidates, vocabulary string) core.Result {
	t.Helper()
	cmd := SelectSubsetBuilder{
		ToolName: "select_subset", Candidates: "$from(choice).names",
		Vocabulary: "$from(declared).values", MatchField: "name",
		AllMatched: "All", Partial: "Partial", Empty: "Empty",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(stubView{
		"choice":   `{"names":` + candidates + `}`,
		"declared": `{"values":` + vocabulary + `}`,
	})
	return cmd.Execute()
}

func TestSelectSubsetSignalsAndTrustBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, candidates, vocabulary string
		signal                       core.Signal
		matched, unmatched           []string
		selected                     string
	}{
		{
			"object vocabulary preserves candidate order",
			`["b","a"]`,
			`[{"name":"a","config":{"value":"first"}},{"name":"b","config":{"value":"second"}}]`,
			"All", []string{"b", "a"}, []string{},
			`[{"name":"b","config":{"value":"second"}},{"name":"a","config":{"value":"first"}}]`,
		},
		{
			"scalar vocabulary",
			`["b","invented","a"]`, `["a","b"]`,
			"Partial", []string{"b", "a"}, []string{"invented"}, `["b","a"]`,
		},
		{
			"duplicate candidates and vocabulary names",
			`["a","a"]`,
			`[{"name":"a","value":"first"},{"name":"a","value":"later"}]`,
			"All", []string{"a", "a"}, []string{},
			`[{"name":"a","value":"first"},{"name":"a","value":"first"}]`,
		},
		{"empty signal", `["invented"]`, `["a"]`, "Empty", []string{}, []string{"invented"}, `[]`},
		{"empty candidates", `[]`, `["a"]`, "Empty", []string{}, []string{}, `[]`},
		{"empty vocabulary", `["a"]`, `[]`, "Empty", []string{}, []string{"a"}, `[]`},
		{"both empty", `[]`, `[]`, "Empty", []string{}, []string{}, `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runSelectSubset(t, tc.candidates, tc.vocabulary)
			require.NoError(t, res.Err)
			require.Equal(t, tc.signal, res.Signal)
			var output struct {
				Matched        []string      `json:"matched"`
				Unmatched      []string      `json:"unmatched"`
				Selected       []interface{} `json:"selected"`
				MatchedCount   int           `json:"matched_count"`
				UnmatchedCount int           `json:"unmatched_count"`
			}
			require.NoError(t, json.Unmarshal([]byte(res.Output), &output))
			var selected []interface{}
			require.NoError(t, json.Unmarshal([]byte(tc.selected), &selected))
			require.Equal(t, tc.matched, output.Matched)
			require.Equal(t, tc.unmatched, output.Unmatched)
			require.Equal(t, selected, output.Selected)
			require.Equal(t, len(tc.matched), output.MatchedCount)
			require.Equal(t, len(tc.unmatched), output.UnmatchedCount)
			for _, name := range output.Matched {
				require.NotEqual(t, "invented", name, "undeclared candidates must never cross the boundary")
			}
		})
	}
}

func TestSelectSubsetRejectsCandidatePayloads(t *testing.T) {
	res := runSelectSubset(
		t,
		`[{"name":"a","untrusted":"payload"}]`,
		`[{"name":"a","trusted":"value"}]`,
	)
	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorContains(t, res.Err, "want string")
	require.NotContains(t, res.Output, `"selected"`)
	require.NotContains(t, res.Output, `"untrusted"`)
}

func TestProjectedStringsFeedSelectSubset(t *testing.T) {
	project := compose.ProjectBuilder{
		ToolName: "project_ids", Items: "$from(rows).items", Field: "id", Signal: "Projected",
	}.Build(core.Result{})
	project.(core.CommandStateAware).SetCommandState(stubView{
		"rows": `{"items":[
			{"id":"b","untrusted":"payload-b"},
			{"id":"invented","untrusted":"payload-invented"},
			{"id":"a","untrusted":"payload-a"}
		]}`,
	})
	projected := project.Execute()
	require.NoError(t, projected.Err)
	require.JSONEq(t, `{"items":["b","invented","a"],"count":3}`, projected.Output)

	selectCmd := SelectSubsetBuilder{
		ToolName: "select_subset", Candidates: "$from(project_ids).items",
		Vocabulary: "$from(declared).values", MatchField: "name",
		AllMatched: "All", Partial: "Partial", Empty: "Empty",
	}.Build(core.Result{})
	selectCmd.(core.CommandStateAware).SetCommandState(stubView{
		"project_ids": projected.Output,
		"declared": `{"values":[
			{"name":"a","trusted":"value-a"},
			{"name":"b","trusted":"value-b"}
		]}`,
	})

	selected := selectCmd.Execute()

	require.NoError(t, selected.Err)
	require.Equal(t, core.Signal("Partial"), selected.Signal)
	require.JSONEq(t, `{
		"matched":["b","a"],
		"unmatched":["invented"],
		"selected":[
			{"name":"b","trusted":"value-b"},
			{"name":"a","trusted":"value-a"}
		],
		"matched_count":2,
		"unmatched_count":1
	}`, selected.Output)
	require.NotContains(t, selected.Output, "untrusted")
}

func TestValidateSelectSubsetConfigRejectsMalformedConfig(t *testing.T) {
	valid := func(candidates, vocabulary, field, all, partial, empty string) error {
		return ValidateSelectSubsetConfig("s", candidates, vocabulary, field, all, partial, empty)
	}
	require.Error(t, valid("$.names", "$from(v).names", "name", "All", "Partial", "Empty"))
	require.Error(t, valid("$from(c).names", "$.names", "name", "All", "Partial", "Empty"))
	require.Error(t, valid("$from(c).names", "$from(v).names", "", "All", "Partial", "Empty"))
	require.Error(t, valid("$from(c).names", "$from(v).names", "name", "", "Partial", "Empty"))
	require.Error(t, valid("$from(c).names", "$from(v).names", "name", "All", "", "Empty"))
	require.Error(t, valid("$from(c).names", "$from(v).names", "name", "All", "Partial", ""))
	require.NoError(t, valid("$from(c).names", "$from(v).names", "name", "All", "Partial", "Empty"))
}
