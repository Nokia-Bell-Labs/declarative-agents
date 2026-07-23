// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the chart schema the executor's validate step rests on (GH-732).
//
// The executor rejects a non-conforming values patch by rendering the chart with
// a helm dry-run, which validates against values.schema.json (srd006 R2.1, AC2).
// That rejection is the executor's entire input-validation story, and
// integration:executor cannot prove it: its fake helm returns whatever exit code
// the scenario sets, so a schema that accepted every document would pass that
// tracer unchanged. Only real helm against the real chart can say whether the
// schema rejects anything.
//
// Substitution, stated because it matters: the executor runs
// `helm upgrade --dry-run`, which needs a reachable cluster --
// "kubernetes cluster unreachable" is as far as it gets here. `helm template`
// validates values against the same values.schema.json without one, so that is
// what these run. The command form the executor itself issues is proven on a
// live cluster by GH-735.

// executorValuesFixture returns a values fixture path from the example's testdata.
func executorValuesFixture(t *testing.T, name string) string {
	t.Helper()
	// findChartDir walks up to the mesh root's helm directory; testdata sits
	// beside it.
	meshRoot := filepath.Dir(findChartDir(t))
	return filepath.Join(meshRoot, "testdata", "integration", "executor-values", name)
}

// renderWithValues renders the chart with one values file, returning helm's
// combined output and whether it succeeded.
func renderWithValues(t *testing.T, values string) (string, bool) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	out, err := exec.Command("helm", "template", "t", findChartDir(t), "-f", values).CombinedOutput()
	return string(out), err == nil
}

// TestChartSchemaAcceptsAConformingPatch proves the schema does not reject the
// values shape the coordinator actually decides. A schema that rejected
// everything would pass the rejection test below while breaking every apply.
func TestChartSchemaAcceptsAConformingPatch(t *testing.T) {
	out, ok := renderWithValues(t, executorValuesFixture(t, "conforming.yaml"))
	if !ok {
		t.Fatalf("the conforming patch did not render:\n%s", out)
	}
	// The patch's own units must appear, or the render succeeded while ignoring
	// the file the test thinks it exercised.
	for _, want := range []string{"t-chatbot-mesh-rag0", "t-chatbot-mesh-rag1"} {
		if !strings.Contains(out, want) {
			t.Errorf("the render does not carry %s; the values file was not applied", want)
		}
	}
}

// TestChartSchemaRejectsANonConformingPatch is the assertion integration:executor
// cannot make. It requires the rejection to name the constraint that caused it:
// helm exiting non-zero proves nothing on its own, since a chart that failed to
// render for an unrelated reason exits non-zero too.
func TestChartSchemaRejectsANonConformingPatch(t *testing.T) {
	out, ok := renderWithValues(t, executorValuesFixture(t, "non-conforming.yaml"))
	if ok {
		t.Fatalf("the non-conforming patch rendered clean; values.schema.json is not enforcing the RAG unit name")
	}
	for _, want := range []string{
		"schema",           // the failure came from schema validation
		"/ragUnits/0/name", // at the field the fixture violates
		"does not match pattern",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the rejection does not mention %q, so it may not be the schema rejecting:\n%s", want, out)
		}
	}
}

// TestChartSchemaConstrainsTheRAGUnitName pins the constraint the fixture rests
// on. Loosening the pattern in values.schema.json must break something, and
// without this it would break only the fixture's rejection -- which reads as a
// test problem rather than as the guard being removed.
func TestChartSchemaConstrainsTheRAGUnitName(t *testing.T) {
	schema, err := readSchemaText(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schema, `"pattern": "^[a-z]([-a-z0-9]*[a-z0-9])?$"`) {
		t.Error("values.schema.json no longer constrains a RAG unit name to a DNS label; " +
			"a name that cannot become a Kubernetes object name would now reach a rollout")
	}
	if !strings.Contains(schema, `"minItems": 1`) {
		t.Error("values.schema.json no longer requires at least one RAG unit; " +
			"a patch emptying the mesh would validate")
	}
}

func readSchemaText(t *testing.T) (string, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(findChartDir(t), "values.schema.json"))
	return string(data), err
}
