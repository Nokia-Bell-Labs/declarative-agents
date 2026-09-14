// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package yamlstrict

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/envexpand"
)

type strictFixture struct {
	Name   string `yaml:"name"`
	Nested struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"nested,omitempty"`
	//nolint:unused // negative fixture for TestTagsOfReturnsExportedYAMLNames: TagsOf and structFields skip unexported fields, and only reflection reads this one.
	ignored string
}

func TestUnmarshalRejectsUnknownField(t *testing.T) {
	t.Parallel()
	var value strictFixture
	err := Unmarshal([]byte("name: ok\ntypo: true\n"), &value)
	require.ErrorContains(t, err, "typo")
}

func TestUnmarshalRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()
	var value strictFixture
	err := Unmarshal([]byte("name: first\n---\nname: second\n"), &value)
	require.ErrorContains(t, err, "multiple YAML documents")
}

func TestUnmarshalAcceptsExpandedEnvironment(t *testing.T) {
	t.Setenv("STRICT_FIXTURE_NAME", "expanded")
	var value strictFixture
	require.NoError(t,
		Unmarshal(envexpand.Expand([]byte("name: ${STRICT_FIXTURE_NAME}\n")), &value))
	require.Equal(t, "expanded", value.Name)
}

func TestCheckFieldsInspectsMergeAliases(t *testing.T) {
	t.Parallel()
	var document yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`
base: &base
  name: inherited
  typo: true
value:
  <<: *base
`), &document))
	value := document.Content[0].Content[3]
	require.ErrorContains(t, CheckFields(value, "name"), "typo")
	require.True(t, FieldPresent(value, "name"))
}

func TestCheckKnownFieldsRejectsNestedUnknownField(t *testing.T) {
	t.Parallel()
	type nested struct {
		Enabled bool `yaml:"enabled"`
	}
	type document struct {
		Config nested `yaml:"config"`
	}
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("config: {enabeld: true}\n"), &node))
	err := CheckKnownFields(node.Content[0], document{})
	require.ErrorContains(t, err, "enabeld")
}

func TestCheckFieldsInspectsDirectMergeMapping(t *testing.T) {
	t.Parallel()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("<<: {name: ok, typo: true}\n"), &node))
	require.ErrorContains(t, CheckFields(node.Content[0], "name"), "typo")
}

func TestCheckFieldsRejectsQuotedMergeLookalike(t *testing.T) {
	t.Parallel()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`"<<": {name: not-a-merge}`+"\n"), &node))
	require.ErrorContains(t, CheckFields(node.Content[0], "name"), `unknown field "<<"`)
}

func TestCheckKnownFieldsInspectsMergeSequence(t *testing.T) {
	t.Parallel()
	type document struct {
		Name string `yaml:"name"`
	}
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`
first: &first {name: inherited}
second: &second {typo: true}
value:
  <<: [*first, *second]
`), &node))
	value := node.Content[0].Content[5]
	require.ErrorContains(t, CheckKnownFields(value, document{}), "typo")
}

func TestTagsOfReturnsExportedYAMLNames(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{"name", "nested"}, TagsOf(strictFixture{}))
	require.Nil(t, TagsOf("not a struct"))
}

func TestUnmarshalRejectsUnexportedFieldName(t *testing.T) {
	t.Parallel()
	var value strictFixture
	require.ErrorContains(t, Unmarshal([]byte("ignored: x\n"), &value), "ignored")
}
