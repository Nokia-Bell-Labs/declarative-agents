// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the agent-architecture chart schema the applier's validate step rests
// on (srd002-applier R2.1, AC2).
//
// The applier rejects a non-conforming values patch by rendering the chart with a
// helm dry-run, which validates against values.schema.json. That rejection is the
// applier's entire input-validation story, and integration:applier cannot prove it:
// its fake helm returns whatever exit code the scenario sets, so a schema that
// accepted every document would pass that tracer unchanged. Only real helm against
// the real chart can say whether the schema rejects anything.
//
// The applier runs `helm upgrade --dry-run`, which needs a reachable cluster. `helm
// template` validates values against the same values.schema.json without one, so
// that is what these run against a prepared chart (the source chart cannot render
// without staged profiles). The command form the applier itself issues is proven on
// a live cluster by integration:applierLive.

// applierValuesFixture returns a values fixture path from the application's testdata.
func applierValuesFixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "testdata", "integration", "applier-values", name)
}

// renderWithValues renders a prepared chart with one values file, returning helm's
// combined output and whether it succeeded. The chart validates values against
// values.schema.json before templating, so a schema rejection surfaces before the
// render.
func renderWithValues(t *testing.T, values string) (string, bool) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := preparedTestChart(t)
	out, err := exec.Command("helm", "template", "t", chart, "-f", values).CombinedOutput()
	return string(out), err == nil
}

// TestChartSchemaAcceptsAConformingPatch proves the schema does not reject the
// values shape an operator actually decides.
func TestChartSchemaAcceptsAConformingPatch(t *testing.T) {
	out, ok := renderWithValues(t, applierValuesFixture(t, "conforming.yaml"))
	if !ok {
		t.Fatalf("the conforming patch did not render:\n%s", out)
	}
	if !strings.Contains(out, "96Mi") {
		t.Errorf("the render does not carry 96Mi; the values file was not applied:\n%s", out)
	}
}

// TestChartSchemaRejectsANonConformingPatch is the assertion integration:applier
// cannot make. It requires the rejection to name the constraint that caused it.
func TestChartSchemaRejectsANonConformingPatch(t *testing.T) {
	out, ok := renderWithValues(t, applierValuesFixture(t, "non-conforming.yaml"))
	if ok {
		t.Fatalf("the non-conforming patch rendered clean; values.schema.json is not enforcing the curator replica bound")
	}
	for _, want := range []string{
		"schema",            // the failure came from schema validation
		"/curator/replicas", // at the field the fixture violates
		"maximum",           // the constraint it violated
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the rejection does not mention %q, so it may not be the schema rejecting:\n%s", want, out)
		}
	}
}

// TestChartSchemaConstrainsCuratorReplicas pins the constraint the fixture rests on.
func TestChartSchemaConstrainsCuratorReplicas(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(findChartDir(t), "values.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	schema := string(data)
	if !strings.Contains(schema, `"maximum": 1`) {
		t.Error("values.schema.json no longer bounds the curator to one replica; " +
			"a patch scaling the single-window demo curator would validate")
	}
	if !strings.Contains(schema, `"const": 18230`) {
		t.Error("values.schema.json no longer pins the applier apply port; " +
			"a drifted port would diverge from the applier rest.yaml listener and the tracer URLs")
	}
}
