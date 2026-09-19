// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package appmanifest

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// variantFixture is a catalog root whose declarations instantiate a fragment
// selected by environment, with the given unit files beside it (GH-2232).
func variantFixture(t *testing.T, fragment string, units ...string) (string, string, Manifest) {
	t.Helper()
	appRoot, catalogRoot, manifest := minimalClosureFixture(t,
		"name: root\ntool_declarations: [declarations.yaml]\n")
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root/declarations.yaml"),
		"unit: root\ninstantiate:\n  - fragment: "+fragment+"\n    args: {stage: embed}\n")
	for _, unit := range units {
		writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root/units", unit), "fragment: "+unit+"\n")
	}
	return appRoot, catalogRoot, manifest
}

func TestResolveStagesEveryVariantOfATemplatedFragment(t *testing.T) {
	appRoot, catalogRoot, manifest := variantFixture(t,
		"units/stage-${X:-a}.yaml", "stage-a.yaml", "stage-b.yaml", "other.yaml")
	inventory, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"agents/root/declarations.yaml",
		"agents/root/profile.yaml",
		"agents/root/units/stage-a.yaml",
		"agents/root/units/stage-b.yaml",
	}
	if got := inventoryRuntimePaths(inventory); !reflect.DeepEqual(got, want) {
		t.Fatalf("closure paths = %v, want %v", got, want)
	}
}

func TestResolveStagesVariantsOfEveryTemplateInOnePath(t *testing.T) {
	appRoot, catalogRoot, manifest := variantFixture(t,
		"units/${P:-ollama}-${S:-embed}.yaml", "ollama-embed.yaml", "cohere-embed.yaml", "cohere-chat.yaml")
	inventory, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(inventoryRuntimePaths(inventory), " ")
	for _, want := range []string{"units/ollama-embed.yaml", "units/cohere-embed.yaml", "units/cohere-chat.yaml"} {
		if !strings.Contains(got, want) {
			t.Errorf("closure lacks %s: %s", want, got)
		}
	}
}

func TestResolveRejectsTemplatedFragmentsItCannotStage(t *testing.T) {
	tests := []struct {
		name     string
		fragment string
		units    []string
		want     []string
	}{
		{"no match", "units/stage-${X:-a}.yaml", []string{"other.yaml"},
			[]string{"units/stage-${X:-a}.yaml", "no file matches"}},
		{"default missing", "units/stage-${X:-a}.yaml", []string{"stage-b.yaml"},
			[]string{"units/stage-${X:-a}.yaml", "default variant units/stage-a.yaml is missing", "units/stage-b.yaml"}},
		{"no default", "units/stage-${X}.yaml", []string{"stage-a.yaml"},
			[]string{"units/stage-${X}.yaml", "needs the form ${NAME:-default}"}},
		{"escaping", "../../../stage-${X:-a}.yaml", []string{"stage-a.yaml"},
			[]string{"stage-${X:-a}.yaml", "escapes ownership root"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appRoot, catalogRoot, manifest := variantFixture(t, test.fragment, test.units...)
			_, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
			for _, want := range test.want {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("error = %v, want %q", err, want)
				}
			}
		})
	}
}

// Only instantiate -> fragment paths expand. A template in any other path field
// keeps today's single manifest_value reading and fails as a dangling path.
func TestResolveKeepsNonFragmentTemplatesUnexpanded(t *testing.T) {
	appRoot, catalogRoot, manifest := minimalClosureFixture(t,
		"name: root\nmachine: machine-${X:-a}.yaml\n")
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root/machine-a.yaml"), "name: machine\n")
	_, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
	if err == nil || !strings.Contains(err.Error(), "machine-manifest_value.yaml") {
		t.Fatalf("error = %v, want the unexpanded manifest_value reading", err)
	}
}

func TestYAMLTemplateTokensRecordTheEnvexpandGrammar(t *testing.T) {
	safe, templates := yamlTemplateTokens([]byte("a: ${A:-x}\nb: ${B}\nc: ${not valid}\n"))
	if string(safe) != "a: manifest_value_t0_\nb: manifest_value_t1_\nc: manifest_value_t2_\n" {
		t.Fatalf("safe = %q", safe)
	}
	want := []closureTemplate{
		{text: "${A:-x}", name: "A", hasDefault: true, defaultVal: "x"},
		{text: "${B}", name: "B"},
		{text: "${not valid}"},
	}
	if !reflect.DeepEqual(templates, want) {
		t.Fatalf("templates = %#v, want %#v", templates, want)
	}
	if got := untokenized("units/x-manifest_value_t0_.yaml"); got != "units/x-manifest_value.yaml" {
		t.Fatalf("untokenized = %q", got)
	}
}
