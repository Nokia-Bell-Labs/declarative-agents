// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package uiyaml

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const schemaPath = "../../applications/ui-kit/schema/ui.v2.schema.json"

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// repositoryUIYAMLs lists every ui.yaml under applications/.
func repositoryUIYAMLs(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir("../../applications", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "dist") {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "ui.yaml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no ui.yaml found; the version 1 compatibility check would pass vacuously")
	}
	return files
}

func TestEveryRepositoryUIYAMLStaysValid(t *testing.T) {
	for _, path := range repositoryUIYAMLs(t) {
		if _, err := Parse(read(t, path)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if err := CheckHelmRenderable(read(t, path)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

func TestVersion2FixtureUsesEveryField(t *testing.T) {
	doc, err := Parse(read(t, "testdata/v2.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.EffectiveVersion() != 2 || len(doc.Panels) != 3 || doc.Branding == nil || doc.TraceBackend == nil ||
		len(doc.MonitoredAgents) != 2 || doc.Panels[1].Config["page_size"] != 25 || !doc.Panels[2].Hidden {
		t.Fatalf("fixture did not decode every version 2 field: %+v", doc)
	}
	if err := CheckHelmRenderable(read(t, "testdata/v2.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejections(t *testing.T) {
	base := string(read(t, "testdata/v2.yaml"))
	cases := []struct{ name, from, to, want string }{
		{"duplicate panel id", "  - id: fleet\n", "  - id: traces\n", `duplicate id "traces"`},
		{"panel id reuses a route id", "  - id: fleet\n", "  - id: help\n", `duplicate id "help"`},
		{"route collision", "    route: /fleet\n", "    route: /traces\n", "route /traces collides"},
		{"panel route collides with a route path", "    route: /fleet\n", "    route: /help\n", "route /help collides"},
		{"undeclared sidebar group", "    sidebar_group: talk\n", "    sidebar_group: nowhere\n", `sidebar_group "nowhere" is not declared`},
		{"panels without version 2", "version: 2\n", "", "panels require version: 2"},
		{"unsupported version", "version: 2\n", "version: 3\n", "version 3 is not supported"},
		{"nested route", "    route: /chat\n", "    route: /a/chat\n", "must be one lower-case segment"},
		{"kit panel without export", "    export: fleet\n", "", "must name the kit panel in export"},
		{"monitored agent listed twice", "  - name: rag0\n", "  - name: chatbot\n", `monitored agent "chatbot" is listed twice`},
		{"trace backend without a name", "  name: collector\n", "  name: \"\"\n", "trace_backend has no name"},
		{"trace query path without the query suffix", "  query_path: /monitor-proxy/collector/query/traces/{trace_id}\n", "  query_path: /monitor-proxy/collector/traces\n", "must be an absolute path ending in /query/traces/{trace_id}"},
		{"relative trace query path", "  query_path: /monitor-proxy/collector/query/traces/{trace_id}\n", "  query_path: query/traces/{trace_id}\n", `query_path "query/traces/{trace_id}" must be an absolute path`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(base, tc.from) {
				t.Fatalf("fixture lacks %q", tc.from)
			}
			_, err := Parse([]byte(strings.Replace(base, tc.from, tc.to, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCheckHelmRenderableRejectsAnchorsAndDocuments(t *testing.T) {
	for name, data := range map[string]string{
		"anchor":        "id: x\nsidebar: &s {title: t}\n",
		"alias":         "id: x\na: &a 1\nb: *a\n",
		"two documents": "id: x\n---\nid: y\n",
	} {
		if err := CheckHelmRenderable([]byte(data)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// The kit's JSON Schema and the Go type describe the same structure: every
// document the Go validator accepts passes the schema, and the schema rejects
// the structural violations.
func TestJSONSchemaAgreesWithTheGoType(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	asJSON := func(data []byte) any {
		var doc any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(strings.NewReader(string(encoded)))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	for _, path := range append(repositoryUIYAMLs(t), "testdata/v2.yaml") {
		if err := schema.Validate(asJSON(read(t, path))); err != nil {
			t.Errorf("%s fails the schema: %v", path, err)
		}
	}
	base := string(read(t, "testdata/v2.yaml"))
	for name, doc := range map[string]string{
		"panels without version 2":                  strings.Replace(base, "version: 2\n", "", 1),
		"nested route":                              strings.Replace(base, "    route: /chat\n", "    route: /a/chat\n", 1),
		"unknown branding key":                      strings.Replace(base, "  accent: \"#005aff\"\n", "  accent: \"#005aff\"\n  font: x\n", 1),
		"panel without package":                     strings.Replace(base, "    package: local\n", "", 1),
		"trace query path without the query suffix": strings.Replace(base, "/collector/query/traces/{trace_id}\n", "/collector/traces\n", 1),
	} {
		if err := schema.Validate(asJSON([]byte(doc))); err == nil {
			t.Errorf("schema accepted %s", name)
		}
	}
}
