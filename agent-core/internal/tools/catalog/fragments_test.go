// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Tool-declaration fragments through the import resolver (srd052 R1, R2).

const embedFragment = `unit: embed-provider
params:
- name: provider
  type: string
  enum: [cohere, openai]
- name: dims
  type: integer
  default: "1024"
tools:
- name: embed
  type: builtin
  init: compose
  description: Embed with $param(provider).
  emits: [Done]
  config:
    provider: $param(provider)
    dims: $param(dims)
`

func writeFragmentFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return root
}

func loadFragmentRoot(t *testing.T, files map[string]string) ([]ToolDef, error) {
	t.Helper()
	root := writeFragmentFixture(t, files)
	return LoadToolDefs(filepath.Join(root, "declarations.yaml"))
}

func TestInstantiateProducesPrefixedTypedTools(t *testing.T) {
	t.Parallel()
	defs, err := loadFragmentRoot(t, map[string]string{
		"units/embed.yaml": embedFragment,
		"declarations.yaml": `unit: rag
instantiate:
- fragment: units/embed.yaml
  as: cohere
  args: {provider: cohere}
- fragment: units/embed.yaml
  as: openai
  args: {provider: openai, dims: 3072}
tools: []
`,
	})
	require.NoError(t, err)
	require.Len(t, defs, 2)

	byName := map[string]ToolDef{}
	for _, def := range defs {
		byName[def.Name] = def
	}
	cohere, openai := byName["cohere_embed"], byName["openai_embed"]
	require.Equal(t, "Embed with cohere.", cohere.Description)
	require.Equal(t, 1024, cohere.Config["dims"], "a whole-scalar integer hole decodes as an integer")
	require.Equal(t, 3072, openai.Config["dims"])
	instantiation, ok := openai.Instantiation()
	require.True(t, ok)
	require.Equal(t, "openai", instantiation.As)
	require.Equal(t, map[string]string{"provider": "openai", "dims": "3072"}, instantiation.Args)
	require.Equal(t, "embed-provider", openai.DeclarationSource().Unit)
}

func TestInstantiateArgumentFaultsNameImporterFragmentAndParameter(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ args, want string }{
		"missing": {"{}", `parameter "provider" is required`},
		"unknown": {"{provider: cohere, model: x}", `argument "model" names no declared parameter`},
		"type":    {"{provider: cohere, dims: many}", `parameter "dims": value "many" is not an integer`},
		"enum":    {"{provider: ollama}", `parameter "provider": value "ollama" is not one of cohere, openai`},
	} {
		_, err := loadFragmentRoot(t, map[string]string{
			"units/embed.yaml":  embedFragment,
			"declarations.yaml": "unit: rag\ninstantiate:\n- fragment: units/embed.yaml\n  args: " + tc.args + "\ntools: []\n",
		})
		require.ErrorContains(t, err, tc.want, name)
		require.ErrorContains(t, err, `tool unit "rag"`, name)
		require.ErrorContains(t, err, `instantiates "units/embed.yaml"`, name)
	}
}

func TestFragmentUnderImportsIsRejected(t *testing.T) {
	t.Parallel()
	_, err := loadFragmentRoot(t, map[string]string{
		"units/embed.yaml":  embedFragment,
		"declarations.yaml": "unit: rag\nimports: [units/embed.yaml]\ntools: []\n",
	})
	require.ErrorContains(t, err, "is instantiated, not imported")
}

func TestPlainUnitCannotBeInstantiated(t *testing.T) {
	t.Parallel()
	_, err := loadFragmentRoot(t, map[string]string{
		"units/plain.yaml":  "unit: plain\ntools:\n- name: x\n  binary: echo\n",
		"declarations.yaml": "unit: rag\ninstantiate:\n- {fragment: units/plain.yaml, args: {}}\ntools: []\n",
	})
	require.ErrorContains(t, err, "declares no params, so it is imported, not instantiated")
}

func TestHalfDeclaredFragmentFails(t *testing.T) {
	t.Parallel()
	_, err := loadFragmentRoot(t, map[string]string{
		"declarations.yaml": "unit: half\nparams:\n- {name: p, type: string}\n",
	})
	require.ErrorContains(t, err, "fragment declares params but no tools or types body")

	_, err = loadFragmentRoot(t, map[string]string{
		"declarations.yaml": "unit: half\ntools:\n- name: x\n  binary: echo\n  description: $param(p)\n",
	})
	require.ErrorContains(t, err, "references $param but declares no params")
}

func TestFragmentCannotNest(t *testing.T) {
	t.Parallel()
	_, err := loadFragmentRoot(t, map[string]string{
		"units/inner.yaml":  embedFragment,
		"units/outer.yaml":  "unit: outer\nparams:\n- {name: p, type: string}\ninstantiate:\n- {fragment: inner.yaml, args: {provider: cohere}}\ntools: []\n",
		"declarations.yaml": "unit: rag\ninstantiate:\n- {fragment: units/outer.yaml, args: {p: x}}\ntools: []\n",
	})
	require.ErrorContains(t, err, "nesting is not supported")
}

// TestReferenceInMappingNameIsLeftAndReported is the hygiene rule: an
// argument cannot mint a field, so the reference survives and is reported.
func TestReferenceInMappingNameIsLeftAndReported(t *testing.T) {
	t.Parallel()
	_, err := loadFragmentRoot(t, map[string]string{
		"units/bad.yaml":    "unit: bad\nparams:\n- {name: p, type: string}\ntools:\n- name: x\n  binary: echo\n  config:\n    $param(p)_key: 1\n",
		"declarations.yaml": "unit: rag\ninstantiate:\n- {fragment: units/bad.yaml, args: {p: v}}\ntools: []\n",
	})
	require.ErrorContains(t, err, "$param( survives substitution")
	require.ErrorContains(t, err, "line 8")
}

func TestRepeatedInstantiationWithoutPrefixIsADuplicate(t *testing.T) {
	t.Parallel()
	_, err := loadFragmentRoot(t, map[string]string{
		"units/embed.yaml":  embedFragment,
		"declarations.yaml": "unit: rag\ninstantiate:\n- {fragment: units/embed.yaml, args: {provider: cohere}}\n- {fragment: units/embed.yaml, args: {provider: cohere}}\ntools: []\n",
	})
	require.ErrorContains(t, err, `duplicate imported tool "embed": instantiations 1 and 2`)
}

// TestInstantiatedUnitTakesTheHandWrittenPath: strict decoding applies to what
// the arguments produced, so a misspelled field in the fragment body fails the
// way it would in any declaration file.
func TestInstantiatedUnitTakesTheHandWrittenPath(t *testing.T) {
	t.Parallel()
	_, err := loadFragmentRoot(t, map[string]string{
		"units/typo.yaml":   "unit: typo\nparams:\n- {name: p, type: string}\ntools:\n- name: x\n  binary: echo\n  descripton: $param(p)\n",
		"declarations.yaml": "unit: rag\ninstantiate:\n- {fragment: units/typo.yaml, args: {p: v}}\ntools: []\n",
	})
	require.ErrorContains(t, err, `unknown field "descripton"`)
}

func TestFragmentImportsResolveFromTheFragment(t *testing.T) {
	t.Parallel()
	defs, err := loadFragmentRoot(t, map[string]string{
		"units/shared.yaml": "unit: shared\ntools:\n- name: shared_tool\n  binary: echo\n",
		"units/frag.yaml":   "unit: frag\nimports: [shared.yaml]\nparams:\n- {name: p, type: string}\ntools:\n- name: own\n  binary: $param(p)\n",
		"declarations.yaml": "unit: rag\ninstantiate:\n- {fragment: units/frag.yaml, as: a, args: {p: echo}}\ntools: []\n",
	})
	require.NoError(t, err)
	names := []string{}
	for _, def := range defs {
		names = append(names, def.Name)
	}
	require.ElementsMatch(t, []string{"shared_tool", "a_own"}, names,
		"the fragment's import is resolved relative to the fragment and merged once")
}

// TestFragmentBodyIsDecodedOnlyAfterSubstitution: a fragment holds $param
// references where typed fields stand, so decoding its body before the
// arguments arrive would reject every integer hole as a string. Only the
// header is decoded until then.
func TestFragmentBodyIsDecodedOnlyAfterSubstitution(t *testing.T) {
	t.Parallel()
	defs, err := loadFragmentRoot(t, map[string]string{
		"units/capped.yaml": "unit: capped\nparams:\n- {name: cap, type: integer}\ntools:\n- name: run\n  binary: echo\n  output_cap: $param(cap)\n",
		"declarations.yaml": "unit: rag\ninstantiate:\n- {fragment: units/capped.yaml, args: {cap: 512}}\ntools: []\n",
	})
	require.NoError(t, err)
	require.Equal(t, 512, defs[0].OutputCap)

	_, err = loadFragmentRoot(t, map[string]string{
		"units/typo.yaml":   "unit: typo\nparams:\n- {name: cap, type: integer}\nimprots: []\ntools: []\n",
		"declarations.yaml": "unit: rag\ninstantiate:\n- {fragment: units/typo.yaml, args: {cap: 1}}\ntools: []\n",
	})
	require.ErrorContains(t, err, `field improts not found`, "the header is still strict")
}
