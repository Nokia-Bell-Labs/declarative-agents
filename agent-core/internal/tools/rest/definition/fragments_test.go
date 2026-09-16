// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// REST-definition fragments through the import resolver (srd052 R1, R2, R3).

const apiFragment = `unit: http-json-api
params:
- {name: base_url, type: string}
- {name: max_bytes, type: integer, default: "4096"}
rest:
  version: v1
  limits:
    local:
      timeout: 5s
      max_request_bytes: $param(max_bytes)
  clients:
    api:
      base_url: $param(base_url)
      limits_ref: local
      operations:
        list: {method: GET, path: /items}
`

func TestInstantiateRESTFragmentTwiceWithPrefixes(t *testing.T) {
	root := t.TempDir()
	fragment := writeImportFixture(t, root, "units/api.yaml", apiFragment)
	top := writeImportFixture(t, root, "rest.yaml", `unit: top
instantiate:
- {fragment: units/api.yaml, as: metrics, args: {base_url: "http://metrics:9090"}}
- {fragment: units/api.yaml, as: traces, args: {base_url: "http://traces:4318", max_bytes: 65536}}
rest: {}
`)

	def, err := LoadDefinitionClosure([]string{top}, nil)

	require.NoError(t, err)
	require.Equal(t, "http://metrics:9090", def.Clients["metrics_api"].BaseURL)
	require.Equal(t, "metrics_local", def.Clients["metrics_api"].LimitsRef,
		"the fragment's own reference follows the renamed limits profile")
	require.Contains(t, def.Clients["metrics_api"].Operations, "metrics_list",
		"operation names are unique across the closure, so they are produced names too")
	require.Equal(t, 4096, def.Limits["metrics_local"].MaxRequestBytes)
	require.Equal(t, 65536, def.Limits["traces_local"].MaxRequestBytes,
		"a whole-scalar integer hole decodes as an integer")
	source, ok := def.DeclarationSource("clients", "traces_api")
	require.True(t, ok)
	require.Equal(t, DeclarationSource{Unit: "http-json-api", Path: fragment}, source)

	instantiations := def.DeclarationInstantiations()
	require.Len(t, instantiations, 2)
	require.Equal(t, "traces", instantiations[1].As)
	require.Equal(t, []string{"clients/traces_api", "limits/traces_local"}, instantiations[1].Produces)
	require.Len(t, def.DeclarationImports(), 2)
	require.Equal(t, map[string]string{"base_url": "http://traces:4318", "max_bytes": "65536"}, def.DeclarationImports()[1].Args)
}

func TestRepeatedRESTInstantiationWithoutPrefixIsTheConflictError(t *testing.T) {
	root := t.TempDir()
	writeImportFixture(t, root, "units/api.yaml", apiFragment)
	top := writeImportFixture(t, root, "rest.yaml", `unit: top
instantiate:
- {fragment: units/api.yaml, args: {base_url: "http://a"}}
- {fragment: units/api.yaml, args: {base_url: "http://b"}}
rest: {}
`)

	_, err := LoadDefinitionClosure([]string{top}, nil)

	require.ErrorContains(t, err, `duplicate REST clients "api"`)
}

func TestRESTArgumentFaultsNameImporterFragmentAndParameter(t *testing.T) {
	root := t.TempDir()
	writeImportFixture(t, root, "units/api.yaml", apiFragment)
	for name, tc := range map[string]struct{ args, want string }{
		"missing": {"{}", `parameter "base_url" is required`},
		"unknown": {`{base_url: "http://a", port: 1}`, `argument "port" names no declared parameter`},
		"type":    {`{base_url: "http://a", max_bytes: big}`, `parameter "max_bytes": value "big" is not an integer`},
	} {
		top := writeImportFixture(t, root, "rest-"+name+".yaml",
			"unit: top-"+name+"\ninstantiate:\n- {fragment: units/api.yaml, args: "+tc.args+"}\nrest: {}\n")
		_, err := LoadDefinitionClosure([]string{top}, nil)
		require.ErrorContains(t, err, tc.want, name)
		require.ErrorContains(t, err, `REST unit "top-`+name+`"`, name)
		require.ErrorContains(t, err, `instantiates "units/api.yaml"`, name)
	}
}

func TestRESTFragmentUnderImportsIsRejected(t *testing.T) {
	root := t.TempDir()
	writeImportFixture(t, root, "units/api.yaml", apiFragment)
	top := writeImportFixture(t, root, "rest.yaml", "unit: top\nimports: [units/api.yaml]\nrest: {}\n")

	_, err := LoadDefinitionClosure([]string{top}, nil)

	require.ErrorContains(t, err, "is instantiated, not imported")
}

func TestHalfDeclaredRESTFragmentFails(t *testing.T) {
	root := t.TempDir()
	paramsOnly := writeImportFixture(t, root, "params-only.yaml", "unit: half\nparams:\n- {name: p, type: string}\n")
	_, err := LoadDefinitionClosure([]string{paramsOnly}, nil)
	require.ErrorContains(t, err, "fragment declares params but no rest body")

	bodyOnly := writeImportFixture(t, root, "body-only.yaml", "unit: half\nrest:\n  clients:\n    api: {base_url: $param(p)}\n")
	_, err = LoadDefinitionClosure([]string{bodyOnly}, nil)
	require.ErrorContains(t, err, "references $param but declares no params")
}

// TestRESTReferenceInMappingNameIsLeftAndReported: an argument cannot mint a
// client, so a reference in a mapping name survives and is reported.
func TestRESTReferenceInMappingNameIsLeftAndReported(t *testing.T) {
	root := t.TempDir()
	writeImportFixture(t, root, "units/bad.yaml", "unit: bad\nparams:\n- {name: p, type: string}\nrest:\n  clients:\n    $param(p)_api: {base_url: x}\n")
	top := writeImportFixture(t, root, "rest.yaml", "unit: top\ninstantiate:\n- {fragment: units/bad.yaml, args: {p: v}}\nrest: {}\n")

	_, err := LoadDefinitionClosure([]string{top}, nil)

	require.ErrorContains(t, err, "$param( survives substitution")
}

func TestRESTFragmentImportsResolveFromTheFragment(t *testing.T) {
	root := t.TempDir()
	writeImportFixture(t, root, "units/shared.yaml", "unit: shared\nrest:\n  auth: {token: {token_ref: T}}\n")
	writeImportFixture(t, root, "units/frag.yaml", "unit: frag\nimports: [shared.yaml]\nparams:\n- {name: u, type: string}\nrest:\n  clients:\n    api: {base_url: $param(u), auth_ref: token}\n")
	top := writeImportFixture(t, root, "rest.yaml", "unit: top\ninstantiate:\n- {fragment: units/frag.yaml, as: a, args: {u: \"http://x\"}}\nrest: {}\n")

	def, err := LoadDefinitionClosure([]string{top}, nil)

	require.NoError(t, err)
	require.Contains(t, def.Auth, "token", "the fragment's import is resolved relative to the fragment")
	require.Equal(t, "token", def.Clients["a_api"].AuthRef,
		"a reference to a name the fragment does not declare is not prefixed")
	require.Equal(t, filepath.Join(root, "units", "frag.yaml"), def.DeclarationImports()[1].Imported.Path)
}
